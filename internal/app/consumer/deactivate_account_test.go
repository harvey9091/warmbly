package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/events"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// stubEmailRepo answers the two calls deactivation makes. Embedding the
// interface keeps the stub to those two; anything else panics loudly rather
// than passing silently.
type stubEmailRepo struct {
	repository.EmailRepository

	workerID    *uuid.UUID
	workerErr   *errx.Error
	updateErr   *errx.Error
	statusSet   []string
	updateCalls int
	workerCalls int
}

func (s *stubEmailRepo) Update(ctx context.Context, userID, emailAccountID string, udata *models.UpdateEmail) (*models.Email, *errx.Error) {
	s.updateCalls++
	if udata.Status != nil {
		s.statusSet = append(s.statusSet, *udata.Status)
	}
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	// Deliberately mirrors the real repository: its RETURNING list carries the
	// mailbox as the dashboard sees it and no worker assignment. Reading
	// WorkerID off this row is what made the removal unreachable the first
	// time, so the stub must keep lying about it in exactly the same way.
	id, _ := uuid.Parse(emailAccountID)
	return &models.Email{ID: id, Status: "inactive"}, nil
}

func (s *stubEmailRepo) GetWorkerID(ctx context.Context, emailAccountID uuid.UUID) (*uuid.UUID, *errx.Error) {
	s.workerCalls++
	return s.workerID, s.workerErr
}

// stubPublisher records the removals published to workers.
type stubPublisher struct {
	events.Publisher

	err     error
	removed []removal
}

type removal struct {
	workerID uuid.UUID
	userID   string
	emailID  string
}

func (p *stubPublisher) PublishRemoveEmail(ctx context.Context, workerID uuid.UUID, remove *models.RemoveWorkerEmail) error {
	p.removed = append(p.removed, removal{workerID: workerID, userID: remove.UserID, emailID: remove.EmailID})
	return p.err
}

func newDeactivationFixture(worker *uuid.UUID) (*JobsService, *stubEmailRepo, *stubPublisher) {
	repo := &stubEmailRepo{workerID: worker}
	pub := &stubPublisher{}
	return &JobsService{EmailRepository: repo, Publisher: pub}, repo, pub
}

func TestDeactivateAccountTellsTheWorkerToDropTheMailbox(t *testing.T) {
	workerID := uuid.New()
	userID := uuid.New()
	emailID := uuid.New()
	s, repo, pub := newDeactivationFixture(&workerID)

	s.deactivateAccount(context.Background(), userID, emailID)

	if len(repo.statusSet) != 1 || repo.statusSet[0] != "inactive" {
		t.Errorf("status writes = %v, want one \"inactive\"", repo.statusSet)
	}
	if len(pub.removed) != 1 {
		t.Fatalf("published %d removals, want 1: the worker keeps syncing a dead mailbox until it restarts", len(pub.removed))
	}
	got := pub.removed[0]
	if got.workerID != workerID {
		t.Errorf("removal sent to worker %s, want %s", got.workerID, workerID)
	}
	if got.userID != userID.String() || got.emailID != emailID.String() {
		t.Errorf("removal carried user=%s email=%s, want user=%s email=%s", got.userID, got.emailID, userID, emailID)
	}
}

// stubWorkerRepo answers only the live-worker listing the eviction needs;
// anything else panics, which is the point.
type stubWorkerRepo struct {
	repository.WorkerRepository
	workers []models.Worker
	err     error
}

func (r *stubWorkerRepo) ListPlaceableWorkers(context.Context) ([]models.Worker, error) {
	return r.workers, r.err
}

func TestDeactivateAccountSkipsAnUnassignedMailbox(t *testing.T) {
	s, repo, pub := newDeactivationFixture(nil)

	s.deactivateAccount(context.Background(), uuid.New(), uuid.New())

	if repo.updateCalls != 1 {
		t.Errorf("update called %d times, want 1", repo.updateCalls)
	}
	if len(pub.removed) != 0 {
		t.Errorf("published %d removals for a mailbox on no worker, want 0", len(pub.removed))
	}
}

// A status write that failed means the mailbox is still active, so publishing
// the removal would strand it: loaded on no worker, active in the database,
// and only the reconciler's next pass to put it back.
func TestDeactivateAccountDoesNotRemoveWhenTheStatusWriteFails(t *testing.T) {
	workerID := uuid.New()
	s, repo, pub := newDeactivationFixture(&workerID)
	repo.updateErr = errx.InternalError()

	s.deactivateAccount(context.Background(), uuid.New(), uuid.New())

	if len(pub.removed) != 0 {
		t.Errorf("published %d removals after a failed status write, want 0", len(pub.removed))
	}
	// The assignment IS read first now, because reading it after the write
	// cannot tell a mailbox that was deleted from one whose write failed.
	// What must not happen is the removal, and that is asserted above.
	if repo.workerCalls != 1 {
		t.Errorf("looked up the assignment %d times, want 1", repo.workerCalls)
	}
}

func TestDeactivateAccountSurvivesAnAssignmentLookupFailure(t *testing.T) {
	s, repo, pub := newDeactivationFixture(nil)
	repo.workerErr = errx.InternalError()

	s.deactivateAccount(context.Background(), uuid.New(), uuid.New())

	if len(pub.removed) != 0 {
		t.Errorf("published %d removals on a failed lookup, want 0", len(pub.removed))
	}
	if repo.updateCalls != 1 {
		t.Errorf("update called %d times, want 1: a lookup that failed is not a reason to leave the mailbox active", repo.updateCalls)
	}
}

// ErrNotFound from the assignment lookup is not a lookup failure: it is the
// mailbox having been deleted. The worker executing it holds it in memory and
// will never be told otherwise, so it authenticates against the provider on
// behalf of an account that no longer exists once per sync interval forever.
// One deleted mailbox produced 444 reports this way before this existed.
func TestDeactivateAccountEvictsAMailboxThatNoLongerExists(t *testing.T) {
	s, repo, pub := newDeactivationFixture(nil)
	repo.workerErr = errx.ErrNotFound
	live := []models.Worker{{ID: uuid.New()}, {ID: uuid.New()}}
	s.WorkerRepo = &stubWorkerRepo{workers: live}

	userID, emailID := uuid.New(), uuid.New()
	s.deactivateAccount(context.Background(), userID, emailID)

	if repo.updateCalls != 0 {
		t.Errorf("update called %d times on a mailbox with no row, want 0", repo.updateCalls)
	}
	if len(pub.removed) != len(live) {
		t.Fatalf("published %d removals, want %d: the assignment died with the row, so every live worker has to be told", len(pub.removed), len(live))
	}
	for i, got := range pub.removed {
		if got.emailID != emailID.String() || got.userID != userID.String() {
			t.Errorf("removal %d carried user=%s email=%s, want user=%s email=%s", i, got.userID, got.emailID, userID, emailID)
		}
	}
}

// With no worker list there is nobody to address the eviction to. It must not
// panic or take the consumer down over a mailbox that is already gone.
func TestDeactivateAccountEvictionSurvivesAnUnavailableWorkerList(t *testing.T) {
	s, repo, pub := newDeactivationFixture(nil)
	repo.workerErr = errx.ErrNotFound
	s.WorkerRepo = &stubWorkerRepo{err: errors.New("db down")}

	s.deactivateAccount(context.Background(), uuid.New(), uuid.New())

	if len(pub.removed) != 0 {
		t.Errorf("published %d removals with no worker list, want 0", len(pub.removed))
	}
}

// A publish failure is logged, not fatal: the account is already inactive and
// the worker drops the mailbox on its next restart.
func TestDeactivateAccountToleratesAPublishFailure(t *testing.T) {
	workerID := uuid.New()
	s, _, pub := newDeactivationFixture(&workerID)
	pub.err = errors.New("bus down")

	s.deactivateAccount(context.Background(), uuid.New(), uuid.New())

	if len(pub.removed) != 1 {
		t.Errorf("published %d removals, want 1 attempt", len(pub.removed))
	}
}

// The consumer runs with pieces unwired in tests and in reduced deployments.
func TestDeactivateAccountWithNothingWired(t *testing.T) {
	(&JobsService{}).deactivateAccount(context.Background(), uuid.New(), uuid.New())

	repo := &stubEmailRepo{}
	(&JobsService{EmailRepository: repo}).deactivateAccount(context.Background(), uuid.New(), uuid.New())
	if repo.updateCalls != 1 {
		t.Errorf("update called %d times with no publisher wired, want 1", repo.updateCalls)
	}
	if repo.workerCalls != 0 {
		t.Errorf("looked up the assignment %d times with no publisher wired, want 0", repo.workerCalls)
	}
}

// Every worker-raised deactivation has to reach the worker, not just the one
// that happened to be debugged. These run the real handlers end to end.
func TestEveryDeactivationPathRemovesTheMailboxFromItsWorker(t *testing.T) {
	handlers := map[string]func(*JobsService, context.Context, models.EmailErrorEvent) error{
		"auth error":   (*JobsService).HandleEmailAuthError,
		"disabled":     (*JobsService).HandleEmailDisabled,
		"rate limited": (*JobsService).HandleEmailRateLimited,
	}

	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			workerID := uuid.New()
			userID := uuid.New()
			emailID := uuid.New()
			s, repo, pub := newDeactivationFixture(&workerID)

			event := models.EmailErrorEvent{
				EmailAccountID: emailID.String(),
				UserID:         userID.String(),
				ErrorCode:      "AUTHENTICATION_FAILED",
				ErrorType:      "critical",
				Message:        "the grant is gone",
			}
			if name == "rate limited" {
				event.ErrorCode = string(errx.MailErrorCodeRateLimitExceeded)
			}
			if err := handle(s, context.Background(), event); err != nil {
				t.Fatalf("handler returned %v", err)
			}

			if len(repo.statusSet) != 1 || repo.statusSet[0] != "inactive" {
				t.Errorf("status writes = %v, want one \"inactive\"", repo.statusSet)
			}
			if len(pub.removed) != 1 {
				t.Fatalf("published %d removals, want 1", len(pub.removed))
			}
			if pub.removed[0].workerID != workerID || pub.removed[0].emailID != emailID.String() {
				t.Errorf("removal = %+v, want worker %s and mailbox %s", pub.removed[0], workerID, emailID)
			}
		})
	}
}

func TestProviderThrottleDoesNotDeactivateMailbox(t *testing.T) {
	for _, code := range []errx.MailErrorCode{
		errx.MailErrorCodeSendingTooFast,
		errx.MailErrorCodeQuotaExceeded,
	} {
		t.Run(string(code), func(t *testing.T) {
			workerID := uuid.New()
			userID := uuid.New()
			emailID := uuid.New()
			s, repo, pub := newDeactivationFixture(&workerID)

			err := s.HandleEmailRateLimited(context.Background(), models.EmailErrorEvent{
				EmailAccountID: emailID.String(),
				UserID:         userID.String(),
				ErrorCode:      string(code),
				ErrorType:      string(errx.MailErrorWarning),
			})
			if err != nil {
				t.Fatalf("handler returned %v", err)
			}
			if repo.updateCalls != 0 || len(pub.removed) != 0 {
				t.Fatalf("provider throttle changed mailbox state: %d updates, %d removals", repo.updateCalls, len(pub.removed))
			}
		})
	}
}

// A malformed event must not be answered with a status write or a removal.
func TestHandlersRejectAMalformedEventBeforeDeactivating(t *testing.T) {
	workerID := uuid.New()
	s, repo, pub := newDeactivationFixture(&workerID)

	err := s.HandleEmailAuthError(context.Background(), models.EmailErrorEvent{
		EmailAccountID: "not-a-uuid",
		UserID:         uuid.New().String(),
	})
	if err == nil {
		t.Error("a malformed email account id was accepted")
	}
	if repo.updateCalls != 0 || len(pub.removed) != 0 {
		t.Errorf("acted on a malformed event: %d updates, %d removals", repo.updateCalls, len(pub.removed))
	}
}
