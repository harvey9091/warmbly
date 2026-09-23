package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// mailboxFKError is what Postgres returns when a write names a mailbox whose
// email_accounts row has been deleted.
func mailboxFKError(table string) error {
	return fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23503", ConstraintName: table + "_email_id_fkey"})
}

// newDeletedMailboxFixture is a consumer with two live workers and a mailbox
// lookup that answers as the given row state.
func newDeletedMailboxFixture(workerID *uuid.UUID, lookupErr *errx.Error) (*JobsService, *stubEmailRepo, *stubPublisher, []models.Worker) {
	workers := []models.Worker{{ID: uuid.New()}, {ID: uuid.New()}}
	repo := &stubEmailRepo{workerID: workerID, workerErr: lookupErr}
	pub := &stubPublisher{}
	return &JobsService{EmailRepository: repo, Publisher: pub, WorkerRepo: &stubWorkerRepo{workers: workers}}, repo, pub, workers
}

func TestDropForDeletedMailboxEvictsFromEveryWorkerWhenTheRowIsGone(t *testing.T) {
	s, _, pub, workers := newDeletedMailboxFixture(nil, errx.ErrNotFound)
	emailID := uuid.New()

	if !s.dropForDeletedMailbox(context.Background(), uuid.New(), emailID, mailboxFKError("unibox_emails")) {
		t.Fatal("a write refused for a deleted mailbox was kept as an error; it is reported and the worker keeps syncing")
	}
	if len(pub.removed) != len(workers) {
		t.Fatalf("published %d removals, want one per live worker (%d)", len(pub.removed), len(workers))
	}
	for i, r := range pub.removed {
		if r.workerID != workers[i].ID || r.emailID != emailID.String() {
			t.Errorf("removal %d = worker %s email %s, want worker %s email %s", i, r.workerID, r.emailID, workers[i].ID, emailID)
		}
	}
}

// A foreign-key refusal alone does not prove the mailbox is gone: the write
// may have tripped the owner's user_id. Evicting a live mailbox would stop it
// syncing, so the row decides.
func TestDropForDeletedMailboxKeepsTheErrorWhileTheMailboxExists(t *testing.T) {
	assigned := uuid.New()
	for name, worker := range map[string]*uuid.UUID{"assigned": &assigned, "unassigned": nil} {
		t.Run(name, func(t *testing.T) {
			s, _, pub, _ := newDeletedMailboxFixture(worker, nil)
			if s.dropForDeletedMailbox(context.Background(), uuid.New(), uuid.New(), mailboxFKError("unibox_emails")) {
				t.Fatal("dropped a refused write for a mailbox that still exists")
			}
			if len(pub.removed) != 0 {
				t.Errorf("published %d removals for a mailbox that still exists", len(pub.removed))
			}
		})
	}
}

func TestDropForDeletedMailboxKeepsTheErrorWhenTheLookupFails(t *testing.T) {
	s, _, pub, _ := newDeletedMailboxFixture(nil, errx.InternalError())
	if s.dropForDeletedMailbox(context.Background(), uuid.New(), uuid.New(), mailboxFKError("unibox_emails")) {
		t.Fatal("dropped the event on a failed lookup; an outage is not a deleted mailbox")
	}
	if len(pub.removed) != 0 {
		t.Errorf("published %d removals on a failed lookup", len(pub.removed))
	}
}

func TestDropForDeletedMailboxLeavesOtherErrorsAlone(t *testing.T) {
	s, repo, _, _ := newDeletedMailboxFixture(nil, errx.ErrNotFound)
	for _, err := range []error{errors.New("connection reset"), &pgconn.PgError{Code: "23505"}, nil} {
		if s.dropForDeletedMailbox(context.Background(), uuid.New(), uuid.New(), err) {
			t.Errorf("dropped %v, which is not a deleted mailbox", err)
		}
	}
	if repo.workerCalls != 0 {
		t.Errorf("looked the mailbox up %d times for errors that are not foreign-key refusals", repo.workerCalls)
	}
}

type refusingMailboxRepo struct {
	repository.MailboxRepository
	err error
}

func (r *refusingMailboxRepo) CreateEntry(context.Context, uuid.UUID, uuid.UUID, *models.Mailbox) error {
	return r.err
}

type refusingSyncStateRepo struct {
	repository.EmailSyncStateRepository
	err error
}

func (r *refusingSyncStateRepo) Get(context.Context, uuid.UUID) (*models.SyncState, error) {
	return nil, nil
}

func (r *refusingSyncStateRepo) Put(context.Context, uuid.UUID, uuid.UUID, *models.SyncState) error {
	return r.err
}

// The handlers are where the refusal used to be discarded or reported; each
// must now end the event and evict.
func TestWorkerEventsForADeletedMailboxEvictInsteadOfFailing(t *testing.T) {
	cases := map[string]func(s *JobsService, userID, emailID uuid.UUID) error{
		"MAILBOX_UPDATE": func(s *JobsService, userID, emailID uuid.UUID) error {
			s.MailboxRepository = &refusingMailboxRepo{err: mailboxFKError("unibox_mailboxes")}
			return s.HandleMailboxUpdate(context.Background(), &models.JobEventMailboxUpdate{UserID: userID, EmailID: emailID, Data: &models.Mailbox{}})
		},
		"SYNC_STATE": func(s *JobsService, userID, emailID uuid.UUID) error {
			s.EmailSyncStateRepository = &refusingSyncStateRepo{err: mailboxFKError("email_sync_state")}
			return s.HandleSyncState(context.Background(), &models.JobEventSyncState{UserID: userID, EmailID: emailID})
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			s, _, pub, workers := newDeletedMailboxFixture(nil, errx.ErrNotFound)
			if err := run(s, uuid.New(), uuid.New()); err != nil {
				t.Fatalf("returned %v; the event is redelivered and reported for a mailbox that cannot come back", err)
			}
			if len(pub.removed) != len(workers) {
				t.Errorf("published %d removals, want %d", len(pub.removed), len(workers))
			}
		})
	}
}
