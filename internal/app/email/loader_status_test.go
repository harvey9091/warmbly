package email

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/worker"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/events"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// stubLoaderRepo serves one mailbox to the loader.
type stubLoaderRepo struct {
	repository.EmailRepository

	account *models.Email
}

// GetSyncSkipFolders answers the loader's skip-list read with an empty list.
func (s *stubLoaderRepo) GetSyncSkipFolders(context.Context, uuid.UUID) ([]string, *errx.Error) {
	return nil, nil
}

func (s *stubLoaderRepo) GetByID(ctx context.Context, emailAccountID uuid.UUID) (*models.Email, *errx.Error) {
	return s.account, nil
}

// countingAssignment records placement attempts. Reaching it at all means the
// loader decided the mailbox belongs on a worker.
type countingAssignment struct {
	worker.WorkerAssignmentService

	assigned int
}

func (c *countingAssignment) IsWorkerLive(ctx context.Context, workerID uuid.UUID) (bool, error) {
	return true, nil
}

func (c *countingAssignment) AssignWorkerToEmail(ctx context.Context, emailAccountID, orgID uuid.UUID) (*uuid.UUID, error) {
	c.assigned++
	id := uuid.New()
	return &id, nil
}

type countingPublisher struct {
	events.Publisher

	added int
}

func (p *countingPublisher) PublishAddEmail(ctx context.Context, workerID uuid.UUID, email *models.AddWorkerEmail) error {
	p.added++
	return nil
}

func loaderFor(status string) (*emailService, *countingAssignment, *countingPublisher) {
	org := uuid.New()
	assign := &countingAssignment{}
	pub := &countingPublisher{}
	repo := &stubLoaderRepo{account: &models.Email{
		ID:             uuid.New(),
		UserID:         uuid.New().String(),
		OrganizationID: &org,
		Email:          "sender@example.com",
		Status:         status,
	}}
	return &emailService{emailRepository: repo, workerAssignment: assign, publisher: pub}, assign, pub
}

// The reconciler lists active mailboxes a tick before it publishes them, so a
// mailbox deactivated in between would otherwise be shipped straight back onto
// the worker the consumer just told to drop it.
func TestLoadAccountOntoWorkerSkipsAMailboxThatIsNotActive(t *testing.T) {
	for _, status := range []string{"inactive", "revoked"} {
		t.Run(status, func(t *testing.T) {
			s, assign, pub := loaderFor(status)

			if err := s.LoadAccountOntoWorker(context.Background(), uuid.New()); err != nil {
				t.Fatalf("LoadAccountOntoWorker: %v", err)
			}
			if assign.assigned != 0 {
				t.Errorf("placed a %s mailbox on a worker (%d assignments)", status, assign.assigned)
			}
			if pub.added != 0 {
				t.Errorf("shipped a %s mailbox to a worker (%d publishes)", status, pub.added)
			}
		})
	}
}

// The guard must not stop the mailboxes that do belong on a worker: an active
// one still reaches placement.
func TestLoadAccountOntoWorkerStillPlacesAnActiveMailbox(t *testing.T) {
	s, assign, _ := loaderFor("active")

	if err := s.LoadAccountOntoWorker(context.Background(), uuid.New()); err != nil {
		t.Fatalf("LoadAccountOntoWorker: %v", err)
	}
	if assign.assigned != 1 {
		t.Errorf("assignments = %d, want 1", assign.assigned)
	}
}
