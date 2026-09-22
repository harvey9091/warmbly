package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type organizationPoolRepo struct {
	repository.WorkerRepository
	ids     []uuid.UUID
	updates map[uuid.UUID]models.WarmupPoolType
}

func (r *organizationPoolRepo) GetEmailAccountsByOrganizationID(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return r.ids, nil
}

func (r *organizationPoolRepo) UpdateEmailAccountWarmupPoolType(_ context.Context, id uuid.UUID, pool models.WarmupPoolType) error {
	r.updates[id] = pool
	return nil
}

func TestSetOrganizationWarmupPoolUpdatesEveryMailbox(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	repo := &organizationPoolRepo{ids: ids, updates: map[uuid.UUID]models.WarmupPoolType{}}
	svc := &workerAssignmentService{workerRepo: repo}

	if err := svc.SetOrganizationWarmupPool(context.Background(), uuid.New(), models.WarmupPoolPremium); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if repo.updates[id] != "premium" {
			t.Errorf("mailbox %s moved to %q, want premium", id, repo.updates[id])
		}
	}
}
