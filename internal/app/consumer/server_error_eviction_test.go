package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// The defect: a worker holding a mailbox whose row is gone calls the provider
// once a sync interval, forever. Its failures classify as server errors
// (wmail maps a provider NotFound into that bucket), and the server-error
// handler deliberately changes no status, so the eviction that the
// deactivation paths perform was unreachable for exactly the case that needs
// it. In production one such mailbox spent a day burning the Gmail API quota
// of a project that no longer had an account to spend it for.
func TestAServerErrorFromADeletedMailboxEvictsItFromEveryWorker(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	repo := &stubEmailRepo{workerErr: errx.ErrNotFound}
	pub := &stubPublisher{}
	s := &JobsService{
		EmailRepository: repo,
		Publisher:       pub,
		WorkerRepo:      &stubWorkerRepo{workers: []models.Worker{{ID: first}, {ID: second}}},
	}

	userID, emailID := uuid.New(), uuid.New()
	if err := s.HandleEmailServerError(context.Background(), models.EmailErrorEvent{
		EmailAccountID: emailID.String(),
		UserID:         userID.String(),
		ErrorCode:      "NOT_FOUND",
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(pub.removed) != 2 {
		t.Fatalf("published %d removals, want one per live worker: the assignment died with the row, so nothing says which one holds it", len(pub.removed))
	}
	for _, got := range pub.removed {
		if got.emailID != emailID.String() || got.userID != userID.String() {
			t.Errorf("removal carried user=%s email=%s, want user=%s email=%s", got.userID, got.emailID, userID, emailID)
		}
	}
	if pub.removed[0].workerID != first || pub.removed[1].workerID != second {
		t.Errorf("removals addressed to %s and %s, want %s and %s", pub.removed[0].workerID, pub.removed[1].workerID, first, second)
	}
}

// A mailbox that still exists must be left alone: a server error is the
// provider being unreachable, which resolves itself, and dropping the mailbox
// off its worker would stop it syncing until the reconciler's next pass.
func TestAServerErrorFromALiveMailboxEvictsNothing(t *testing.T) {
	workerID := uuid.New()
	repo := &stubEmailRepo{workerID: &workerID}
	pub := &stubPublisher{}
	s := &JobsService{
		EmailRepository: repo,
		Publisher:       pub,
		WorkerRepo:      &stubWorkerRepo{workers: []models.Worker{{ID: uuid.New()}}},
	}

	if err := s.HandleEmailServerError(context.Background(), models.EmailErrorEvent{
		EmailAccountID: uuid.New().String(),
		UserID:         uuid.New().String(),
		ErrorCode:      "SERVER_UNREACHABLE",
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(pub.removed) != 0 {
		t.Errorf("published %d removals for a mailbox that still exists, want 0", len(pub.removed))
	}
}
