package emailverify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	verify "github.com/warmbly/warmbly/internal/pkg/emailverify"
	"github.com/warmbly/warmbly/internal/repository"
)

type providerSource struct {
	ProviderSource
	provider *Provider

	mu       sync.Mutex
	reported error
	// onReport stands in for a slow health write, so a check resolving
	// alongside one can be caught overtaking it.
	onReport func()
}

func (s *providerSource) VerificationProviderFor(context.Context, uuid.UUID) (*Provider, error) {
	return s.provider, nil
}
func (s *providerSource) ReportVerificationProviderError(_ context.Context, _ uuid.UUID, err error) {
	s.mu.Lock()
	s.reported = err
	s.mu.Unlock()
	if s.onReport != nil {
		s.onReport()
	}
}
func (s *providerSource) ClearVerificationProviderError(context.Context, uuid.UUID) {
	s.mu.Lock()
	s.reported = nil
	s.mu.Unlock()
}
func (s *providerSource) lastReport() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reported
}

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
	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != verify.ProviderBuiltin || source.lastReport() == nil {
		t.Fatalf("fallback = %+v, report = %v", res, source.lastReport())
	}
	overview, err = s.Overview(context.Background(), org)
	if err != nil || overview.Provider != verify.ProviderBuiltin || overview.ProviderError == "" {
		t.Fatalf("empty-account overview = %+v, %v", overview, err)
	}
	status = http.StatusOK
	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != "cleanmylist" || source.lastReport() != nil {
		t.Fatalf("recovered account = %+v, report = %v", res, source.lastReport())
	}
	overview, err = s.Overview(context.Background(), org)
	if err != nil || overview.Provider != "cleanmylist" || overview.ProviderError != "" {
		t.Fatalf("recovered overview = %+v, %v", overview, err)
	}
}

// An exhausted CleanMyList account answers the account check exactly like a
// healthy one, so the 402 a real check found is the only evidence there is. It
// has to outlive the ordinary lookup cache, or every pass puts the whole batch
// back on doomed paid calls a minute later.
func TestExhaustedAccountWithoutBalanceIsHeldPastTheLookupCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/jobs" {
			_, _ = w.Write([]byte(`{"jobs":[]}`))
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer srv.Close()
	id := uuid.New()
	source := &providerSource{provider: &Provider{Name: "cleanmylist", Label: "CleanMyList", ConnectionID: &id, Client: verify.NewCleanMyList("key", srv.URL)}}
	s := NewService(contactCountsRepo{}, Options{Builtin: builtinVerifier{}, Providers: source}).(*service)
	org := uuid.New()

	if res := s.VerifyAddress(context.Background(), org, "test@example.com"); res.Provider != verify.ProviderBuiltin {
		t.Fatalf("result = %+v", res)
	}
	s.creditsMu.Lock()
	for k, e := range s.credits {
		e.at, e.until = e.at.Add(-2*time.Minute), e.until.Add(-2*time.Minute)
		s.credits[k] = e
	}
	s.creditsMu.Unlock()

	overview, err := s.Overview(context.Background(), org)
	if err != nil || overview.Provider != verify.ProviderBuiltin || overview.ProviderError == "" {
		t.Fatalf("two minutes on, the empty account = %+v, %v", overview, err)
	}
	if !strings.Contains(overview.ProviderError, "CleanMyList") {
		t.Fatalf("provider error does not name the service: %q", overview.ProviderError)
	}
}

// Checks run concurrently and resolve out of order, so one that started before
// a failure must not withdraw it: it never saw it.
func TestOlderCheckDoesNotClearANewerFailure(t *testing.T) {
	id := uuid.New()
	source := &providerSource{}
	s := NewService(contactCountsRepo{}, Options{Builtin: builtinVerifier{}, Providers: source}).(*service)
	p := &Provider{Name: "cleanmylist", ConnectionID: &id, Client: verify.NewCleanMyList("key", "http://127.0.0.1:1")}

	startedBefore := time.Now()
	time.Sleep(time.Millisecond)
	s.noteProviderError(context.Background(), p, verify.ErrProviderCredits)
	if source.lastReport() == nil {
		t.Fatal("the failure was not reported")
	}

	s.noteProviderOK(context.Background(), p, startedBefore)
	if !s.providerDown(p) || source.lastReport() == nil {
		t.Fatal("a check that began before the failure cleared it")
	}
	s.noteProviderOK(context.Background(), p, time.Now())
	if s.providerDown(p) || source.lastReport() != nil {
		t.Fatal("a check that began after the failure did not clear it")
	}
}

// The health write is a round trip, so a check that resolves while one is in
// flight must not withdraw it. Recording the failure before marking the
// connection is what gives that check something to compare itself against.
func TestASuccessResolvingMidReportDoesNotUndoTheFailure(t *testing.T) {
	id := uuid.New()
	reporting := make(chan struct{})
	source := &providerSource{onReport: func() {
		close(reporting)
		time.Sleep(50 * time.Millisecond)
	}}
	s := NewService(contactCountsRepo{}, Options{Builtin: builtinVerifier{}, Providers: source}).(*service)
	p := &Provider{Name: "cleanmylist", ConnectionID: &id, Client: verify.NewCleanMyList("key", "http://127.0.0.1:1")}

	startedBefore := time.Now()
	time.Sleep(time.Millisecond)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.noteProviderError(context.Background(), p, verify.ErrProviderCredits)
	}()
	<-reporting
	s.noteProviderOK(context.Background(), p, startedBefore)
	<-done

	if source.lastReport() == nil {
		t.Fatal("a check that began before the failure withdrew it mid-report")
	}
	if !s.providerDown(p) {
		t.Fatal("the failure was not held")
	}
}
