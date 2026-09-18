package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/events"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// backfillPublisher records the warmup actions the sweep sends.
type backfillPublisher struct {
	events.Publisher
	actions []models.WarmupEmailAction
	workers []uuid.UUID
}

func (p *backfillPublisher) PublishWarmupAction(_ context.Context, workerID uuid.UUID, a *models.WarmupEmailAction) error {
	p.actions = append(p.actions, *a)
	p.workers = append(p.workers, workerID)
	return nil
}

// backfillEmailRepo hands back one mailbox with a chosen filing preference.
type backfillEmailRepo struct {
	repository.EmailRepository
	account *models.Email
}

func (r backfillEmailRepo) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	return r.account, nil
}

// TestCleanupFilesTheLeakOutOfTheCustomersOwnMailbox covers the half of #583
// the sweep never did: deleting the Unibox row hides warmup mail from our
// inbox, but the copy in the customer's Gmail or Outlook is the one they are
// looking at, and nothing else ever goes back for it.
func TestCleanupFilesTheLeakOutOfTheCustomersOwnMailbox(t *testing.T) {
	worker := uuid.New()
	e := models.JobEventNewEmail{UserID: uuid.New(), Message: &models.EmailMessageStoreData{
		ID: uuid.New(), EmailID: uuid.New(), MessageID: "<leak@test.local>",
		GmailID: "gm-1", UID: 42, Mailbox: 7, FolderPath: "INBOX",
	}}
	inbox := &warmupCleanupInbox{events: []models.JobEventNewEmail{e}}
	pub := &backfillPublisher{}
	s := &JobsService{
		UniboxRepository: inbox,
		CloudLink:        &warmupInboxCloud{deliveryOK: true},
		Publisher:        pub,
		EmailRepository: backfillEmailRepo{account: &models.Email{
			ID: e.Message.EmailID, WorkerID: &worker,
			WarmupPlacement: models.WarmupPlacementFolder, WarmupFolder: "Warmbly",
		}},
	}

	if _, _, err := s.cleanWarmupInboxBatch(context.Background(), uuid.Nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(inbox.deleted) != 1 {
		t.Fatalf("unibox row not removed: %d", len(inbox.deleted))
	}
	if len(pub.actions) != 1 {
		t.Fatalf("the leak was hidden from Unibox but left in the customer's mailbox: %d actions published", len(pub.actions))
	}
	got := pub.actions[0]
	if pub.workers[0] != worker {
		t.Errorf("published to worker %s, want %s", pub.workers[0], worker)
	}
	if len(got.Actions) != 1 || got.Actions[0] != models.WarmupActionFile {
		t.Errorf("actions = %v, want only %q: replaying engagement on old mail is activity no real reader produces",
			got.Actions, models.WarmupActionFile)
	}
	// The Gmail path acts on the provider id, so the locators have to survive
	// the trip from the Unibox row into the action.
	if got.GmailID != "gm-1" || got.RFCMessageID != "<leak@test.local>" || got.MailboxFolder != "INBOX" {
		t.Errorf("locators lost: gmail=%q rfc=%q folder=%q", got.GmailID, got.RFCMessageID, got.MailboxFolder)
	}
	if got.Placement != models.WarmupPlacementFolder || got.TargetFolder != "Warmbly" {
		t.Errorf("filing = %q/%q, want folder/Warmbly", got.Placement, got.TargetFolder)
	}
}

// TestCleanupLeavesWarmupAloneWhenTheOwnerAskedForInbox: placement "inbox" is a
// deliberate choice, so there is no leak to repair and moving the mail would
// override the setting.
func TestCleanupLeavesWarmupAloneWhenTheOwnerAskedForInbox(t *testing.T) {
	worker := uuid.New()
	e := models.JobEventNewEmail{UserID: uuid.New(), Message: &models.EmailMessageStoreData{
		ID: uuid.New(), EmailID: uuid.New(), MessageID: "<keep@test.local>",
	}}
	inbox := &warmupCleanupInbox{events: []models.JobEventNewEmail{e}}
	pub := &backfillPublisher{}
	s := &JobsService{
		UniboxRepository: inbox,
		CloudLink:        &warmupInboxCloud{deliveryOK: true},
		Publisher:        pub,
		EmailRepository: backfillEmailRepo{account: &models.Email{
			ID: e.Message.EmailID, WorkerID: &worker,
			WarmupPlacement: models.WarmupPlacementInbox,
		}},
	}

	if _, _, err := s.cleanWarmupInboxBatch(context.Background(), uuid.Nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(pub.actions) != 0 {
		t.Fatalf("moved mail the owner asked to keep in the inbox: %+v", pub.actions)
	}
	if len(inbox.deleted) != 1 {
		t.Fatalf("unibox row should still be removed: %d", len(inbox.deleted))
	}
}
