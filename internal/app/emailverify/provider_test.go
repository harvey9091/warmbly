package emailverify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	verify "github.com/warmbly/warmbly/internal/pkg/emailverify"
	"github.com/warmbly/warmbly/internal/repository"
)

type providerSource struct {
	ProviderSource
	provider *Provider
	reported error
}

func (s *providerSource) VerificationProviderFor(context.Context, uuid.UUID) (*Provider, error) {
	return s.provider, nil
}
func (s *providerSource) ReportVerificationProviderError(_ context.Context, _ uuid.UUID, err error) {
	s.reported = err
}
func (s *providerSource) ClearVerificationProviderError(context.Context, uuid.UUID) { s.reported = nil }

type contactCountsRepo struct{ repository.ContactRepository }

func (contactCountsRepo) VerificationCounts(context.Context, uuid.UUID) (models.ContactVerificationCounts, *errx.Error) {
	return models.ContactVerificationCounts{}, nil
}

type builtinVerifier struct{}

func (builtinVerifier) Verify(_ context.Context, email string) verify.Result {
	return verify.Result{Email: email, Status: verify.StatusUnknown, Provider: verify.ProviderBuiltin}
}

func TestCleanMyListProviderWithoutBalance(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/jobs" {
			_, _ = w.Write([]byte(`{"jobs":[]}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"verdict":"risky","reason_code":"catch_all"}`))
	}))
	defer srv.Close()
	id := uuid.New()
	source := &providerSource{provider: &Provider{Name: "cleanmylist", ConnectionID: &id, Client: verify.NewCleanMyList("key", srv.URL)}}
	s := NewService(contactCountsRepo{}, Options{Builtin: builtinVerifier{}, Providers: source})
	org := uuid.New()
	overview, err := s.Overview(context.Background(), org)
	if err != nil || overview.Provider != "cleanmylist" || overview.Credits != nil || overview.ProviderError != "" {
		t.Fatalf("overview = %+v, %v", overview, err)
	}
	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != "cleanmylist" || res.Status != verify.StatusRisky {
		t.Fatalf("result = %+v", res)
	}
	status = http.StatusPaymentRequired
	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != verify.ProviderBuiltin || source.reported == nil {
		t.Fatalf("fallback = %+v, report = %v", res, source.reported)
	}
	overview, err = s.Overview(context.Background(), org)
	if err != nil || overview.Provider != verify.ProviderBuiltin || overview.ProviderError == "" {
		t.Fatalf("empty-account overview = %+v, %v", overview, err)
	}
	status = http.StatusOK
	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != "cleanmylist" || source.reported != nil {
		t.Fatalf("recovered account = %+v, report = %v", res, source.reported)
	}
	overview, err = s.Overview(context.Background(), org)
	if err != nil || overview.Provider != "cleanmylist" || overview.ProviderError != "" {
		t.Fatalf("recovered overview = %+v, %v", overview, err)
	}
}
