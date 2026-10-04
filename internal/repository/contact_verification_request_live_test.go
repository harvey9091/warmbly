package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emailverify"
)

// A member's re-verify has to reach the verifier whatever the contact's
// history: a verdict fresh from a paid provider, real mail seen last week, a
// manual verdict. It also has to leave the current verdict standing until
// the new one lands, so campaigns keep routing on it meanwhile.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/<db>?sslmode=disable \
//	  go test ./internal/repository/ -run LiveVerificationRequest -v
func TestLiveVerificationRequestReachesTheVerifier(t *testing.T) {
	handle, pool := liveContactDB(t)
	requireSchemaVersion(t, pool, 199)
	f := newSharedOrgFixture(t, pool)
	ctx := context.Background()
	repo := &contactRepository{DB: handle}
	evidence := &verificationEvidenceRepository{DB: handle}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	contact := func(status, source, provider string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		exec(`INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone,
		          custom_fields, updated_at, created_at,
		          verification_status, verification_provider, verification_source, verification_checked_at,
		          verification_evidence_at)
		      VALUES ($1, $2, $3, $4, 'Ada', 'Ng', '', '', '{}'::jsonb, NOW(), NOW(),
		          $5, $6, $7, NOW() - INTERVAL '1 day', NOW() - INTERVAL '7 days')`,
			id, f.owner, f.org, "request-"+id.String()[:8]+"@test.local", status, provider, source)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM contacts WHERE id = $1`, id)
		})
		return id
	}
	candidate := func(id uuid.UUID) *VerificationCandidate {
		t.Helper()
		got, xerr := repo.ListVerificationCandidates(ctx, 500)
		if xerr != nil {
			t.Fatalf("list candidates: %v", xerr)
		}
		for i := range got {
			if got[i].ID == id {
				return &got[i]
			}
		}
		return nil
	}
	status := func(id uuid.UUID) (string, string, *time.Time) {
		t.Helper()
		var s, check string
		var requested *time.Time
		if err := pool.QueryRow(ctx, `SELECT verification_status, verification_check_status, verification_requested_at FROM contacts WHERE id = $1`, id).
			Scan(&s, &check, &requested); err != nil {
			t.Fatalf("read contact: %v", err)
		}
		return s, check, requested
	}

	// Checked yesterday by the paid verifier, with real mail seen last week:
	// the scheduler has no reason to touch it on its own.
	paid := contact("valid", "provider", "millionverifier")
	manual := contact("valid", "manual", "manual")
	if candidate(paid) != nil || candidate(manual) != nil {
		t.Fatal("a fresh verdict is a candidate nobody asked for")
	}

	n, xerr := repo.RequestContactsVerification(ctx, f.org, []uuid.UUID{paid, manual})
	if xerr != nil || n != 2 {
		t.Fatalf("request = %d, %v", n, xerr)
	}
	if s, _, requested := status(paid); s != "valid" || requested == nil {
		t.Fatalf("the request replaced the verdict (%q) or was not recorded (%v)", s, requested)
	}
	c := candidate(paid)
	if c == nil || c.RequestedAt == nil {
		t.Fatal("a requested re-check is not a candidate")
	}
	if candidate(manual) == nil {
		t.Fatal("a requested re-check of a manual verdict is not a candidate")
	}

	// A request that lands while the check runs is newer than it and stays.
	asked := *c.RequestedAt
	exec(`UPDATE contacts SET verification_requested_at = NOW() + INTERVAL '1 second' WHERE id = $1`, paid)
	res := emailverify.Result{Status: emailverify.StatusValid, Provider: emailverify.ProviderMillionVerifier, Reason: "real mail"}
	if xerr := repo.UpdateContactVerification(ctx, paid, res, emailverify.StatusInvalid, &asked); xerr != nil {
		t.Fatalf("update: %v", xerr)
	}
	if _, check, requested := status(paid); requested == nil || check != "invalid" {
		t.Fatalf("a newer request was cleared (%v) or the check's own answer was lost (%q)", requested, check)
	}

	// The check that answers the current request clears it.
	c = candidate(paid)
	if xerr := repo.UpdateContactVerification(ctx, paid, res, emailverify.StatusInvalid, c.RequestedAt); xerr != nil {
		t.Fatalf("update: %v", xerr)
	}
	if _, _, requested := status(paid); requested != nil {
		t.Fatal("the answered request was not cleared")
	}
	if candidate(paid) != nil {
		t.Fatal("an answered request is still a candidate")
	}

	// Scoring starts from what the check said, not from the stored override.
	v, err := evidence.Verdict(ctx, paid)
	if err != nil || v.Status != emailverify.StatusInvalid || v.Stored != emailverify.StatusValid || v.Provider != emailverify.ProviderMillionVerifier {
		t.Fatalf("verdict = %+v, %v", v, err)
	}

	// A verdict a member sets answers any re-check still waiting.
	if _, xerr := repo.SetContactsVerification(ctx, f.org, []uuid.UUID{manual}, models.ContactVerificationWrite{
		Status: "valid", Reason: "marked", Provider: "manual", Source: models.VerificationSourceManual,
	}); xerr != nil {
		t.Fatalf("set: %v", xerr)
	}
	if _, _, requested := status(manual); requested != nil {
		t.Fatal("a manual verdict left the re-check waiting")
	}

	// A manual verdict set while a requested check runs is not overwritten.
	if _, xerr := repo.RequestContactsVerification(ctx, f.org, []uuid.UUID{manual}); xerr != nil {
		t.Fatalf("request: %v", xerr)
	}
	inFlight := candidate(manual)
	if _, xerr := repo.SetContactsVerification(ctx, f.org, []uuid.UUID{manual}, models.ContactVerificationWrite{
		Status: "invalid", Reason: "marked", Provider: "manual", Source: models.VerificationSourceManual,
	}); xerr != nil {
		t.Fatalf("set: %v", xerr)
	}
	_ = repo.UpdateContactVerification(ctx, manual, res, emailverify.StatusValid, inFlight.RequestedAt)
	if s, _, _ := status(manual); s != "invalid" {
		t.Fatalf("a check that was in flight overwrote the manual verdict with %q", s)
	}

	// A lapsed override falls back to what the row says the check answered,
	// even when the rescore read an older check.
	exec(`UPDATE contacts SET verification_status = 'valid', verification_check_status = 'invalid' WHERE id = $1`, paid)
	if err := evidence.SetScore(ctx, paid, "valid", 40, "stale read", time.Time{}, false); err != nil {
		t.Fatalf("set score: %v", err)
	}
	if s, _, _ := status(paid); s != "invalid" {
		t.Fatalf("a non-decisive rescore wrote %q over the check's own verdict", s)
	}

	// Requests take at most half a batch while the backlog has work.
	var queued []uuid.UUID
	for i := 0; i < 3; i++ {
		queued = append(queued, contact("valid", "provider", "millionverifier"))
	}
	fresh := contact("unknown", "", "")
	exec(`UPDATE contacts SET verification_checked_at = NULL, verification_evidence_at = NULL WHERE id = $1`, fresh)
	if _, xerr := repo.RequestContactsVerification(ctx, f.org, queued); xerr != nil {
		t.Fatalf("request: %v", xerr)
	}
	got, xerr := repo.ListVerificationCandidates(ctx, 2)
	if xerr != nil || len(got) != 2 {
		t.Fatalf("batch = %v, %v", got, xerr)
	}
	if got[0].RequestedAt == nil || got[1].RequestedAt != nil {
		t.Fatalf("a bulk re-verify took the whole batch from the backlog: %+v", got)
	}

	// Another workspace cannot queue checks on this one's contacts.
	if n, _ := repo.RequestContactsVerification(ctx, uuid.New(), []uuid.UUID{paid}); n != 0 {
		t.Fatalf("a foreign organization queued %d checks", n)
	}
}
