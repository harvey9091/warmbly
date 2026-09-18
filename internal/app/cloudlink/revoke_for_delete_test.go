package cloudlink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// stubLinkRepo is the local half of the link. Embedding the interface keeps it
// to the calls the revocation makes; anything else panics loudly.
type stubLinkRepo struct {
	repository.CloudLinkRepository

	link    *models.CloudLink
	linkErr error

	mailbox    *models.CloudLinkMailbox
	mailboxErr error

	unenrolled  []uuid.UUID
	unenrollErr error
}

func (r *stubLinkRepo) Get(context.Context) (*models.CloudLink, error) {
	return r.link, r.linkErr
}

func (r *stubLinkRepo) GetByAccount(context.Context, uuid.UUID) (*models.CloudLinkMailbox, error) {
	return r.mailbox, r.mailboxErr
}

func (r *stubLinkRepo) Unenroll(_ context.Context, accountID uuid.UUID) error {
	r.unenrolled = append(r.unenrolled, accountID)
	return r.unenrollErr
}

// stubEmails answers the ownership check.
type stubEmails struct {
	repository.EmailRepository

	account *models.Email
}

func (s stubEmails) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	return s.account, nil
}

type revokeFixture struct {
	svc     *service
	repo    *stubLinkRepo
	org     uuid.UUID
	account uuid.UUID
	// deletes records the paths the cloud was asked to delete.
	deletes *[]string
}

// newRevokeFixture stands up the service against a fake cloud that answers
// whatever status the test asks for.
func newRevokeFixture(t *testing.T, cloudStatus int) *revokeFixture {
	t.Helper()
	org, account := uuid.New(), uuid.New()
	deletes := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			*deletes = append(*deletes, r.URL.Path)
		}
		w.WriteHeader(cloudStatus)
		switch {
		case cloudStatus == http.StatusNotFound:
			// What the cloud's own handler answers for a mailbox it does not
			// hold (poollink.ErrMailboxNotFound), which is the one refusal the
			// revocation is allowed to read as success.
			_, _ = w.Write([]byte(`{"code":"pool_link_mailbox_not_found","message":"That mailbox is not enrolled."}`))
		case cloudStatus >= 400:
			_, _ = w.Write([]byte(`{"code":"internal_error","message":"nope"}`))
		}
	}))
	t.Cleanup(srv.Close)

	repo := &stubLinkRepo{
		link:    &models.CloudLink{CloudURL: srv.URL, Token: "t"},
		mailbox: &models.CloudLinkMailbox{EmailAccountID: account, RemoteID: account},
	}
	svc := &service{
		repo:     repo,
		emails:   stubEmails{account: &models.Email{ID: account, OrganizationID: &org}},
		sessions: map[string]oauthSession{},
		tokens:   map[uuid.UUID]cachedToken{},
	}
	return &revokeFixture{svc: svc, repo: repo, org: org, account: account, deletes: deletes}
}

// The nil answer is the whole contract: the caller deletes the mailbox on the
// strength of it, and afterwards there is no record left to retry from.
func TestRevokeForDeleteTakesTheMailboxOffTheCloudBeforeTheLocalRow(t *testing.T) {
	f := newRevokeFixture(t, http.StatusNoContent)

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr != nil {
		t.Fatalf("RevokeForDelete: %v", xerr)
	}
	if len(*f.deletes) != 1 || (*f.deletes)[0] != "/v1/pool-link/instance/mailboxes/"+f.account.String() {
		t.Fatalf("cloud deletes = %v, want the mailbox's own path", *f.deletes)
	}
	if len(f.repo.unenrolled) != 1 || f.repo.unenrolled[0] != f.account {
		t.Fatalf("local rows dropped = %v, want one for %s", f.repo.unenrolled, f.account)
	}
}

// A cloud that refuses must not be reported as a revocation, or the mailbox is
// deleted and its password stays in the pool for good.
func TestRevokeForDeleteRefusesWhenTheCloudDoes(t *testing.T) {
	f := newRevokeFixture(t, http.StatusInternalServerError)

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr == nil {
		t.Fatal("a refused cloud delete was reported as a revocation")
	}
	if len(f.repo.unenrolled) != 0 {
		t.Errorf("the local row was dropped although the cloud still holds the mailbox: %v", f.repo.unenrolled)
	}
}

// An unreadable link is not an absent one. Get errors when the stored token
// cannot be decrypted, and treating that as "not linked" would skip the cloud
// entirely and answer nil.
func TestRevokeForDeleteRefusesAnUnreadableLink(t *testing.T) {
	f := newRevokeFixture(t, http.StatusNoContent)
	f.repo.link, f.repo.linkErr = nil, errors.New("cannot decrypt the instance token")

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr == nil {
		t.Fatal("an unreadable link was read as a mailbox the cloud does not hold")
	}
	if len(*f.deletes) != 0 || len(f.repo.unenrolled) != 0 {
		t.Errorf("acted on an unreadable link: deletes=%v unenrolled=%v", *f.deletes, f.repo.unenrolled)
	}
}

// A mailbox the cloud already let go of is a success, not a failure: the
// delete has to be able to finish after a half-completed earlier attempt.
func TestRevokeForDeleteToleratesAMailboxTheCloudHasAlreadyDropped(t *testing.T) {
	f := newRevokeFixture(t, http.StatusNotFound)

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr != nil {
		t.Fatalf("a mailbox the cloud no longer holds was refused: %v", xerr)
	}
}

// An instance with no link at all holds nothing in any pool, and a mailbox
// that was never enrolled needs no round trip.
func TestRevokeForDeleteIsANoopWithNothingToRevoke(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*revokeFixture)
	}{
		{"never enrolled", func(f *revokeFixture) { f.repo.mailbox = nil }},
		{"instance not linked", func(f *revokeFixture) { f.repo.link = nil }},
	} {
		f := newRevokeFixture(t, http.StatusNoContent)
		tc.setup(f)
		if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr != nil {
			t.Fatalf("%s: %v", tc.name, xerr)
		}
		if len(*f.deletes) != 0 {
			t.Errorf("%s: called the cloud anyway: %v", tc.name, *f.deletes)
		}
	}
}

// The local row failing to go is not a refusal: the cloud has already let go,
// and the mailbox row takes its link with it by cascade.
func TestRevokeForDeleteSucceedsEvenIfTheLocalRowCannotBeDropped(t *testing.T) {
	f := newRevokeFixture(t, http.StatusNoContent)
	f.repo.unenrollErr = errors.New("db down")

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr != nil {
		t.Fatalf("revocation refused over local bookkeeping: %v", xerr)
	}
	if len(*f.deletes) != 1 {
		t.Fatalf("cloud deletes = %v, want one", *f.deletes)
	}
}

// A mailbox from another workspace is refused before anything is asked of the cloud.
func TestRevokeForDeleteRefusesAForeignMailbox(t *testing.T) {
	f := newRevokeFixture(t, http.StatusNoContent)
	other := uuid.New()
	f.svc.emails = stubEmails{account: &models.Email{ID: f.account, OrganizationID: &other}}

	if xerr := f.svc.RevokeForDelete(context.Background(), f.org, f.account); xerr == nil {
		t.Fatal("revoked an enrollment for a mailbox in another workspace")
	}
	if len(*f.deletes) != 0 {
		t.Errorf("called the cloud for a foreign mailbox: %v", *f.deletes)
	}
}
