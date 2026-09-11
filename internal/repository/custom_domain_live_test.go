package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Issue #400: the on-demand TLS gate decides whether a hostname gets a
// certificate, so the query behind it has to be right against the real schema
// rather than against a mock that agrees with it.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveCustomDomain -v
func TestLiveCustomDomainIsVerified(t *testing.T) {
	_, pool := liveContactDB(t)
	f := newSharedOrgFixture(t, pool)
	ctx := context.Background()
	repo := NewCustomDomainRepository(pool)

	suffix := uuid.New().String()[:8]
	formsDomain := "f-" + suffix + ".test.local"
	campaignDomain := "c-" + suffix + ".test.local"

	// A domain that is stored but not verified must NOT get a certificate: it
	// is never built into a link, so nothing legitimate asks for it.
	if _, err := pool.Exec(ctx,
		`UPDATE organizations SET forms_domain = $2, forms_domain_verified = false WHERE id = $1`,
		f.org, formsDomain); err != nil {
		t.Fatalf("set unverified forms domain: %v", err)
	}
	assertVerified(t, repo, formsDomain, false)

	if _, err := pool.Exec(ctx,
		`UPDATE organizations SET forms_domain_verified = true WHERE id = $1`, f.org); err != nil {
		t.Fatalf("verify forms domain: %v", err)
	}
	assertVerified(t, repo, formsDomain, true)

	// A campaign-level tracking override is its own arm of the query, and was
	// the one most likely to be forgotten.
	if _, err := pool.Exec(ctx,
		`UPDATE campaigns SET tracking_domain = $2, tracking_domain_verified = true WHERE id = $1`,
		f.campaign, campaignDomain); err != nil {
		t.Fatalf("set campaign tracking domain: %v", err)
	}
	assertVerified(t, repo, campaignDomain, true)

	// Nothing else on the instance, and the empty string in particular: every
	// row that has no custom domain stores '', so a blank SNI must not match
	// all of them at once.
	assertVerified(t, repo, "nobody-"+suffix+".test.local", false)
	assertVerified(t, repo, "", false)
}

func assertVerified(t *testing.T, repo CustomDomainRepository, host string, want bool) {
	t.Helper()
	got, err := repo.IsVerified(context.Background(), host)
	if err != nil {
		t.Fatalf("IsVerified(%q): %v", host, err)
	}
	if got != want {
		t.Fatalf("IsVerified(%q) = %v, want %v", host, got, want)
	}
}
