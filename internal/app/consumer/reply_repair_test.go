package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type repairInbox struct {
	repository.UniboxRepository
	events []models.JobEventNewEmail
	since  time.Time
}

func (r *repairInbox) ListUnprocessedCampaignReplies(_ context.Context, since time.Time, afterID uuid.UUID, limit int) ([]models.JobEventNewEmail, error) {
	r.since = since
	var out []models.JobEventNewEmail
	for _, e := range r.events {
		if e.Message.ID.String() > afterID.String() {
			out = append(out, e)
		}
	}
	return out[:min(len(out), limit)], nil
}

type repairAdvanced struct {
	replyRecordingAdvanced
	failOn uuid.UUID
	seen   []uuid.UUID
}

func (s *repairAdvanced) ProcessIncomingReply(_ context.Context, _ uuid.UUID, m *models.EmailMessageStoreData) *errx.Error {
	s.seen = append(s.seen, m.ID)
	if m.ID == s.failOn {
		return errx.InternalError()
	}
	return nil
}

// Every unclaimed reply is re-offered to the same handler the live path uses,
// a failure on one does not stop the rest, and the cursor lands on the last
// message so the next batch continues rather than repeats.
func TestIncomingReplyRepairReoffersEveryUnclaimedReply(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	var events []models.JobEventNewEmail
	for _, id := range ids {
		events = append(events, models.JobEventNewEmail{UserID: uuid.New(), Message: &models.EmailMessageStoreData{ID: id, EmailID: uuid.New(), MessageID: "<" + id.String() + "@test>"}})
	}
	inbox := &repairInbox{events: events}
	adv := &repairAdvanced{failOn: ids[1]}
	s := &JobsService{UniboxRepository: inbox, AdvancedService: adv}

	next, done, err := s.repairIncomingReplyBatch(context.Background(), uuid.Nil)
	if err != nil || !done {
		t.Fatalf("batch: next=%v done=%v err=%v", next, done, err)
	}
	if len(adv.seen) != 3 {
		t.Fatalf("re-offered %d replies, want all 3 (a failure must not stop the sweep)", len(adv.seen))
	}
	// The cursor is the highest id seen, whichever order the fake returned.
	var last uuid.UUID
	for _, id := range ids {
		if id.String() > last.String() {
			last = id
		}
	}
	if next != last {
		t.Fatalf("cursor = %v, want the last message %v", next, last)
	}
	if time.Since(inbox.since) > replyRepairWindow+time.Minute || time.Since(inbox.since) < replyRepairWindow-time.Minute {
		t.Fatalf("asked since %v, want the repair window", inbox.since)
	}
}

func TestIncomingReplyRepairNeedsBothCollaborators(t *testing.T) {
	// Neither a nil inbox nor a nil advanced service may panic the consumer.
	(&JobsService{}).StartIncomingReplyRepair(context.Background())
	(&JobsService{UniboxRepository: &repairInbox{}}).StartIncomingReplyRepair(context.Background())
	_ = errors.New
}
