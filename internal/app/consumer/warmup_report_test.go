package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// warmupReportRepo answers IsWarmupDelivery for the id the worker said the
// report was about, and records what it was asked.
type warmupReportRepo struct {
	repository.WarmupRepository

	// warmup is keyed by the message id asked about, so a stub answer cannot
	// stand in for the wrong question: ingest asks about the arrival's OWN id
	// before it asks about the send a report names, and a blanket true would
	// make this pass without isWarmupReport running at all.
	warmup   map[string]bool
	err      error
	askedID  string
	askedFor uuid.UUID
	calls    int
}

// A report carries no warmup token, so the paths ahead of isWarmupReport all
// have to answer "not warmup" before it is reached.
func (r *warmupReportRepo) FindDeliveredWarmupToken(context.Context, uuid.UUID, string, string, string) (*models.WarmupToken, error) {
	return nil, nil
}

func (r *warmupReportRepo) IsWarmupDelivery(_ context.Context, accountID uuid.UUID, _, messageID, _ string) (bool, error) {
	r.calls++
	r.askedID = messageID
	r.askedFor = accountID
	return r.warmup[messageID], r.err
}

func TestIsWarmupReport(t *testing.T) {
	mailbox := uuid.New()
	event := func(about string) *models.JobEventNewEmail {
		return &models.JobEventNewEmail{
			Message:                 &models.EmailMessageStoreData{EmailID: mailbox, Folder: models.FolderInbox},
			ReportOriginalMessageID: about,
		}
	}

	t.Run("a bounce about a warmup send is kept out", func(t *testing.T) {
		repo := &warmupReportRepo{warmup: map[string]bool{"warm-1@sender.test": true}}
		s := &JobsService{WarmupRepo: repo}
		got, err := s.isWarmupReport(context.Background(), event("warm-1@sender.test"))
		if err != nil || !got {
			t.Fatalf("isWarmupReport = (%v, %v), want (true, nil)", got, err)
		}
		if repo.askedID != "warm-1@sender.test" || repo.askedFor != mailbox {
			t.Fatalf("asked about (%q, %v), want the send the worker named on this mailbox", repo.askedID, repo.askedFor)
		}
	})

	t.Run("a bounce about a campaign send is the customer's to see", func(t *testing.T) {
		s := &JobsService{WarmupRepo: &warmupReportRepo{}}
		got, err := s.isWarmupReport(context.Background(), event("camp-1@sender.test"))
		if err != nil || got {
			t.Fatalf("isWarmupReport = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("ordinary mail is never looked up", func(t *testing.T) {
		repo := &warmupReportRepo{warmup: map[string]bool{"warm-1@sender.test": true}}
		s := &JobsService{WarmupRepo: repo}
		got, err := s.isWarmupReport(context.Background(), event(""))
		if err != nil || got {
			t.Fatalf("isWarmupReport = (%v, %v), want (false, nil)", got, err)
		}
		if repo.calls != 0 {
			t.Fatalf("a message that is not a report cost %d lookups, want 0", repo.calls)
		}
	})

	t.Run("a failed lookup holds the arrival rather than filing it", func(t *testing.T) {
		s := &JobsService{WarmupRepo: &warmupReportRepo{err: errors.New("connection reset by peer")}}
		got, err := s.isWarmupReport(context.Background(), event("warm-1@sender.test"))
		if err == nil {
			t.Fatal("a lookup failure must be surfaced, not read as 'not warmup'")
		}
		if got {
			t.Fatal("a failed lookup must not claim the report is warmup")
		}
	})
}

// A report about a warmup send must not reach the unibox at all.
func TestHandleNewEmailKeepsWarmupBounceOutOfInbox(t *testing.T) {
	mailbox := uuid.New()
	inbox := &warmupInboxRepo{}
	s := &JobsService{
		WarmupRepo:       &warmupReportRepo{warmup: map[string]bool{"warm-1@sender.test": true}},
		UniboxRepository: inbox,
		EmailRepository:  warmupInboxEmailRepo{},
	}
	err := s.HandleNewEmail(context.Background(), &models.JobEventNewEmail{
		UserID: uuid.New(),
		Message: &models.EmailMessageStoreData{
			EmailID: mailbox,
			Folder:  models.FolderInbox,
			Subject: "Undelivered Mail Returned to Sender",
		},
		ReportOriginalMessageID: "warm-1@sender.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inbox.entries != 0 {
		t.Fatal("a bounce notice for a warmup send was filed in the customer's unibox")
	}
}
