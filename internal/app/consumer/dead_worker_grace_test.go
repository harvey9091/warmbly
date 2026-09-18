package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type graceWorkerRepo struct {
	repository.WorkerRepository
	worker *models.Worker
}

func (r graceWorkerRepo) GetByID(context.Context, uuid.UUID) (*models.Worker, error) {
	return r.worker, nil
}

// TestEvacuationWaitsOutARestart is the guard on the migration in #583. The
// heartbeat key lives three minutes and an auto-update replaces the container
// well inside that, so evacuating the moment the key expires moved 92
// mailboxes off a worker that was healthy again seconds later. Each of those
// moves changes the address the mailbox's provider sees.
func TestEvacuationWaitsOutARestart(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		name     string
		lastSeen *time.Time
		want     bool
	}{
		{
			name:     "mid restart",
			lastSeen: ptrTime(time.Now().Add(-90 * time.Second)),
			want:     false,
		},
		{
			name:     "just inside the grace",
			lastSeen: ptrTime(time.Now().Add(-(MailboxEvacuationGrace - time.Minute))),
			want:     false,
		},
		{
			name:     "genuinely gone",
			lastSeen: ptrTime(time.Now().Add(-(MailboxEvacuationGrace + time.Minute))),
			want:     true,
		},
		{
			name:     "never seen is unknown age, not old",
			lastSeen: nil,
			want:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &JobsService{WorkerRepo: graceWorkerRepo{worker: &models.Worker{
				ID: id, LastSeenAt: tc.lastSeen,
			}}}
			if got := s.unreachableLongEnoughToEvacuate(context.Background(), models.Worker{ID: id}); got != tc.want {
				t.Fatalf("unreachableLongEnoughToEvacuate = %v, want %v", got, tc.want)
			}
		})
	}
}

// A worker row that cannot be re-read is not evidence of death: the scan's
// snapshot is stale by the time it reaches this worker, so an unreadable row
// must not authorise moving mailboxes.
func TestEvacuationRefusesWithoutAFreshRow(t *testing.T) {
	s := &JobsService{WorkerRepo: graceWorkerRepo{worker: nil}}
	if s.unreachableLongEnoughToEvacuate(context.Background(), models.Worker{ID: uuid.New()}) {
		t.Fatal("evacuated on an unreadable worker row")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
