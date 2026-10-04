package organization

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// Only what Create touches is implemented; anything else panics.
type createRepo struct {
	repository.OrganizationRepository
	created *models.Organization
}

func (r *createRepo) GetUserOwnedOrganizationCount(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}

func (r *createRepo) Create(_ context.Context, org *models.Organization) error {
	r.created = org
	return nil
}

func (r *createRepo) AddMember(context.Context, *models.OrganizationMember) error { return nil }

func (r *createRepo) CreateRole(context.Context, *models.OrganizationRole) error { return nil }

type createUsers struct {
	repository.UserRepository
}

func (createUsers) GetBanState(context.Context, uuid.UUID) (uint32, error) { return 0, nil }

func (createUsers) GetUser(_ context.Context, id uuid.UUID) (*models.User, error) {
	return &models.User{ID: id, MaxOrganizations: 5}, nil
}

// The zone a new workspace is created with is the one it keeps.
func TestCreateStoresTheWorkspaceTimezone(t *testing.T) {
	repo := &createRepo{}
	svc := &organizationService{orgRepo: repo, userRepo: createUsers{}}

	org, xerr := svc.Create(context.Background(), uuid.New(), "Acme", " America/New_York ")
	if xerr != nil {
		t.Fatalf("Create: %v", xerr)
	}
	if repo.created == nil || repo.created.Timezone != "America/New_York" || org.Timezone != "America/New_York" {
		t.Fatalf("stored %+v, returned %q; want America/New_York", repo.created, org.Timezone)
	}

	if _, xerr := svc.Create(context.Background(), uuid.New(), "Acme", "Mars/Olympus"); xerr == nil || xerr != errx.ErrTimezone {
		t.Fatalf("an unknown zone was accepted: %v", xerr)
	}
}
