package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Deleting an API key removes the row and its usage logs, and refuses a key
// that can still authenticate: ending a live credential is Revoke's job
// (issue #414).
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveAPIKeyDelete -v

type apiKeyFixture struct {
	org   uuid.UUID
	other uuid.UUID
	owner uuid.UUID
}

func newAPIKeyFixture(t *testing.T, pool *pgxpool.Pool) *apiKeyFixture {
	t.Helper()
	ctx := context.Background()
	f := &apiKeyFixture{org: uuid.New(), other: uuid.New(), owner: uuid.New()}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	tag := f.org.String()[:8]

	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Keys', 'Live', $2, 'x')`, f.owner, "keys-"+tag+"@fixture.invalid")
	for i, org := range []uuid.UUID{f.org, f.other} {
		exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
		      VALUES ($1, 'Keys', $2, $3)`, org, "keys-"+tag+"-"+string(rune('a'+i)), f.owner)
		exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
		      VALUES ($1, $2, 'owner', NOW())`, org, f.owner)
	}

	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM api_keys WHERE organization_id = ANY($1)`, []uuid.UUID{f.org, f.other}},
			{`DELETE FROM organization_members WHERE organization_id = ANY($1)`, []uuid.UUID{f.org, f.other}},
			{`DELETE FROM organizations WHERE id = ANY($1)`, []uuid.UUID{f.org, f.other}},
			{`DELETE FROM users WHERE id = $1`, f.owner},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

// addKey inserts one key and returns its id.
func (f *apiKeyFixture) addKey(t *testing.T, pool *pgxpool.Pool, org uuid.UUID, status string, expiresAt *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, organization_id, name, key_prefix, key_suffix, key_hash, permissions, status, expires_at)
		 VALUES ($1, $2, $3, 'Live key', 'wmbly_ab', 'wxyz', $4, 1, $5, $6)`,
		id, f.owner, org, id.String(), status, expiresAt)
	if err != nil {
		t.Fatalf("insert key: %v", err)
	}
	return id
}

func TestLiveAPIKeyDelete(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newAPIKeyFixture(t, pool)
	repo := NewAPIKeyRepository(handle)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	t.Run("a revoked key is deleted with its usage logs", func(t *testing.T) {
		id := f.addKey(t, pool, f.org, "revoked", nil)
		if _, err := pool.Exec(ctx,
			`INSERT INTO api_key_usage_logs (api_key_id, endpoint, method, ip_address, response_status, response_time_ms)
			 VALUES ($1, '/v1/campaigns', 'GET', '203.0.113.7', 200, 12)`, id); err != nil {
			t.Fatalf("insert usage log: %v", err)
		}
		if xerr := repo.Delete(ctx, f.org, id); xerr != nil {
			t.Fatalf("delete: %v", xerr)
		}
		if _, xerr := repo.GetByID(ctx, f.org, id); xerr != errx.ErrNotFound {
			t.Fatalf("key still readable after delete: %v", xerr)
		}
		var logs int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_key_usage_logs WHERE api_key_id = $1`, id).Scan(&logs); err != nil {
			t.Fatalf("count logs: %v", err)
		}
		if logs != 0 {
			t.Fatalf("usage logs left behind: %d", logs)
		}
	})

	t.Run("an active key is refused", func(t *testing.T) {
		id := f.addKey(t, pool, f.org, "active", nil)
		if xerr := repo.Delete(ctx, f.org, id); xerr != errx.ErrNotFound {
			t.Fatalf("delete of an active key = %v, want not found", xerr)
		}
		if _, xerr := repo.GetByID(ctx, f.org, id); xerr != nil {
			t.Fatalf("active key was deleted anyway: %v", xerr)
		}
	})

	t.Run("an unexpired key is refused", func(t *testing.T) {
		id := f.addKey(t, pool, f.org, "active", &future)
		if xerr := repo.Delete(ctx, f.org, id); xerr != errx.ErrNotFound {
			t.Fatalf("delete of an unexpired key = %v, want not found", xerr)
		}
	})

	// The status column only ever holds 'active' or 'revoked'; expiry is
	// applied when the key is read, so a key past expires_at is already dead
	// and deleting it is allowed.
	t.Run("a key past its expiry is deleted", func(t *testing.T) {
		id := f.addKey(t, pool, f.org, "active", &past)
		if xerr := repo.Delete(ctx, f.org, id); xerr != nil {
			t.Fatalf("delete of an expired key: %v", xerr)
		}
	})

	t.Run("another organization's key is not found", func(t *testing.T) {
		id := f.addKey(t, pool, f.other, "revoked", nil)
		if xerr := repo.Delete(ctx, f.org, id); xerr != errx.ErrNotFound {
			t.Fatalf("cross-org delete = %v, want not found", xerr)
		}
		if _, xerr := repo.GetByID(ctx, f.other, id); xerr != nil {
			t.Fatalf("another org's key was deleted: %v", xerr)
		}
	})

	t.Run("deleting twice is not found the second time", func(t *testing.T) {
		id := f.addKey(t, pool, f.org, "revoked", nil)
		if xerr := repo.Delete(ctx, f.org, id); xerr != nil {
			t.Fatalf("first delete: %v", xerr)
		}
		if xerr := repo.Delete(ctx, f.org, id); xerr != errx.ErrNotFound {
			t.Fatalf("second delete = %v, want not found", xerr)
		}
	})
}

// The dashboard's key-count strip has to agree with the status the list shows
// for the same key: one past its expires_at is expired, not active.
func TestLiveAPIKeyUsageSummaryCountsExpiry(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newAPIKeyFixture(t, pool)
	repo := NewAPIKeyRepository(handle)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	f.addKey(t, pool, f.org, "active", nil)
	f.addKey(t, pool, f.org, "active", &future)
	f.addKey(t, pool, f.org, "active", &past)
	f.addKey(t, pool, f.org, "revoked", nil)

	sum, xerr := repo.GetUsageSummary(ctx, f.org)
	if xerr != nil {
		t.Fatalf("usage summary: %v", xerr)
	}
	if sum.ActiveKeys != 2 {
		t.Errorf("active_keys = %d, want 2", sum.ActiveKeys)
	}
	if sum.ExpiredKeys != 1 {
		t.Errorf("expired_keys = %d, want 1", sum.ExpiredKeys)
	}
	if sum.RevokedKeys != 1 {
		t.Errorf("revoked_keys = %d, want 1", sum.RevokedKeys)
	}
}

func TestAPIKeyCanAuthenticate(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	cases := []struct {
		name string
		key  models.APIKey
		want bool
	}{
		{"active, no expiry", models.APIKey{Status: models.APIKeyStatusActive}, true},
		{"active, expires later", models.APIKey{Status: models.APIKeyStatusActive, ExpiresAt: &future}, true},
		{"active, already expired", models.APIKey{Status: models.APIKeyStatusActive, ExpiresAt: &past}, false},
		{"revoked", models.APIKey{Status: models.APIKeyStatusRevoked}, false},
		{"revoked, expires later", models.APIKey{Status: models.APIKeyStatusRevoked, ExpiresAt: &future}, false},
		{"expired", models.APIKey{Status: models.APIKeyStatusExpired}, false},
	}
	for _, tc := range cases {
		if got := tc.key.CanAuthenticate(); got != tc.want {
			t.Errorf("%s: CanAuthenticate() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
