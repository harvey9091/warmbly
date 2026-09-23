package emailverify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	verify "github.com/warmbly/warmbly/internal/pkg/emailverify"
	"github.com/warmbly/warmbly/internal/repository"
)

// requestRepo holds one contact a member asked to re-verify.
type requestRepo struct {
	repository.ContactRepository
	org, id   uuid.UUID
	requested *time.Time

	mu      sync.Mutex
	stored  *verify.Result
	check   verify.Status
	cleared *time.Time
}

func (r *requestRepo) RequestContactsVerification(_ context.Context, orgID uuid.UUID, ids []uuid.UUID) (int, *errx.Error) {
	if orgID != r.org || len(ids) != 1 || ids[0] != r.id {
		return 0, nil
	}
	now := time.Now().UTC()
	r.requested = &now
	return 1, nil
}

func (r *requestRepo) ListVerificationCandidates(context.Context, int) ([]repository.VerificationCandidate, *errx.Error) {
	if r.requested == nil {
		return nil, nil
	}
	return []repository.VerificationCandidate{{ID: r.id, OrganizationID: r.org, Email: "dana@acme.com", RequestedAt: r.requested}}, nil
}

func (r *requestRepo) UpdateContactVerification(_ context.Context, _ uuid.UUID, res verify.Result, check verify.Status, requestedAt *time.Time) *errx.Error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stored, r.check, r.cleared = &res, check, requestedAt
	return nil
}

func millionVerifierServer(t *testing.T, credits string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/credits" {
			_, _ = w.Write([]byte(credits))
			return
		}
		_, _ = w.Write([]byte(`{"email":"dana@acme.com","result":"invalid","resultcode":6,"subresult":"user_unknown","role":false,"error":""}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Re-verify has to reach the connected verifier and say so, and the check has
// to hand the request it answers back to be cleared.
func TestReverifyRunsThroughTheConnectedVerifier(t *testing.T) {
	srv := millionVerifierServer(t, `{"credits":0,"renewing_credits":900}`)
	conn := uuid.New()
	source := &providerSource{provider: &Provider{Name: verify.ProviderMillionVerifier, Label: "MillionVerifier", ConnectionID: &conn, Client: verify.NewMillionVerifier("key", srv.URL)}}
	repo := &requestRepo{org: uuid.New(), id: uuid.New()}
	s := NewService(repo, Options{Builtin: builtinVerifier{}, Providers: source})

	resp, xerr := s.Request(context.Background(), repo.org, models.ContactVerificationRequest{
		ContactSelection: models.ContactSelection{Contacts: []string{repo.id.String()}},
		Action:           models.ContactVerificationActionVerify,
	})
	if xerr != nil || resp.Affected != 1 || !resp.Queued {
		t.Fatalf("request = %+v, %v", resp, xerr)
	}
	if resp.Verifier != verify.ProviderMillionVerifier || resp.VerifierLabel != "MillionVerifier" || resp.VerifierError != "" {
		t.Fatalf("a subscription account was not used: %+v", resp)
	}

	if n, xerr := s.VerifyPending(context.Background(), 10); xerr != nil || n != 1 {
		t.Fatalf("pass = %d, %v", n, xerr)
	}
	if repo.stored == nil || repo.stored.Provider != verify.ProviderMillionVerifier || repo.stored.Status != verify.StatusInvalid {
		t.Fatalf("stored = %+v", repo.stored)
	}
	if repo.check != verify.StatusInvalid || repo.cleared == nil || !repo.cleared.Equal(*repo.requested) {
		t.Fatalf("check status %q, cleared %v, requested %v", repo.check, repo.cleared, repo.requested)
	}
}

// A connected verifier that cannot be used is named in the response, so the
// member knows the built-in check is running instead.
func TestReverifySaysWhyTheVerifierIsPassedOver(t *testing.T) {
	srv := millionVerifierServer(t, `{"credits":0,"renewing_credits":0}`)
	conn := uuid.New()
	source := &providerSource{provider: &Provider{Name: verify.ProviderMillionVerifier, Label: "MillionVerifier", ConnectionID: &conn, Client: verify.NewMillionVerifier("key", srv.URL)}}
	repo := &requestRepo{org: uuid.New(), id: uuid.New()}
	s := NewService(repo, Options{Builtin: builtinVerifier{}, Providers: source})

	resp, xerr := s.Request(context.Background(), repo.org, models.ContactVerificationRequest{
		ContactSelection: models.ContactSelection{Contacts: []string{repo.id.String()}},
		Action:           models.ContactVerificationActionVerify,
	})
	if xerr != nil || resp.Verifier != verify.ProviderBuiltin || resp.VerifierError == "" {
		t.Fatalf("response = %+v, %v", resp, xerr)
	}
}
