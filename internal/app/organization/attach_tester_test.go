package organization

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// Only the reads AttachTester makes before it writes are implemented; anything
// else panics, which is what keeps this honest about the path it covers.
type attachRepo struct {
	repository.OrganizationRepository
	org    *models.Organization
	member *models.OrganizationMember
	role   *models.OrganizationRole
	added  int
}

func (r *attachRepo) GetByID(context.Context, uuid.UUID) (*models.Organization, error) {
	return r.org, nil
}

func (r *attachRepo) GetMember(context.Context, uuid.UUID, uuid.UUID) (*models.OrganizationMember, error) {
	return r.member, nil
}

func (r *attachRepo) GetRoleByID(context.Context, uuid.UUID, uuid.UUID) (*models.OrganizationRole, error) {
	return r.role, nil
}

func (r *attachRepo) AddMemberWithRoles(_ context.Context, m *models.OrganizationMember, _ []uuid.UUID) error {
	r.added++
	r.member = m
	return nil
}

func attachSvc(r *attachRepo) *organizationService {
	return &organizationService{orgRepo: r}
}

// A workspace that does not exist must not silently become one, because the
// caller has already made the account by this point.
func TestAttachTesterRefusesAMissingWorkspace(t *testing.T) {
	r := &attachRepo{org: nil}
	_, xerr := attachSvc(r).AttachTester(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	if xerr == nil {
		t.Fatal("a missing workspace was accepted")
	}
	if xerr.Code != errx.NotFound {
		t.Fatalf("code = %v, want NotFound", xerr.Code)
	}
	if r.added != 0 {
		t.Fatalf("wrote %d memberships for a workspace that does not exist", r.added)
	}
}

// The role is the whole of what a tester can reach. A role that does not
// resolve must fail rather than fall back to one, and must not reach the write.
func TestAttachTesterRefusesAnUnknownRole(t *testing.T) {
	r := &attachRepo{org: &models.Organization{ID: uuid.New()}, role: nil}
	_, xerr := attachSvc(r).AttachTester(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	if xerr == nil {
		t.Fatal("an unknown role was accepted")
	}
	if xerr.Code != errx.BadRequest {
		t.Fatalf("code = %v, want BadRequest", xerr.Code)
	}
	if r.added != 0 {
		t.Fatalf("wrote %d memberships for a role that does not exist", r.added)
	}
}

// Repeating the call must not re-role an existing member. A tester left on a
// narrow role should stay on it, and a second call is how that would quietly
// widen.
func TestAttachTesterLeavesAnExistingMembershipAlone(t *testing.T) {
	orgID, userID := uuid.New(), uuid.New()
	existing := &models.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           "Viewer",
		Permissions:    models.PermViewCampaigns,
	}
	r := &attachRepo{
		org:    &models.Organization{ID: orgID},
		member: existing,
		role:   &models.OrganizationRole{ID: uuid.New(), Name: "Admin", Permissions: models.AllPermissions},
	}
	got, xerr := attachSvc(r).AttachTester(context.Background(), orgID, userID, uuid.New(), r.role.ID)
	if xerr != nil {
		t.Fatalf("unexpected error: %v", xerr)
	}
	if r.added != 0 {
		t.Fatalf("wrote %d memberships for a user who was already a member", r.added)
	}
	if got.Role != "Viewer" || got.Permissions != models.PermViewCampaigns {
		t.Fatalf("membership was widened to %s/%d", got.Role, got.Permissions)
	}
}
