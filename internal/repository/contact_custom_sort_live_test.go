package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/models"
)

// The contacts list sorts on a custom field with sort_by "custom:<key>". The
// value is text in jsonb, nullable by nature (most rows lack most fields), so
// the keyset cursor has to walk the NULL block correctly in both directions and
// SearchIDs has to hand back the rows in the order the list showed them.
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveContactCustomFieldSort -v

type customSortFixture struct {
	org   uuid.UUID
	user  uuid.UUID
	total int
}

func newCustomSortFixture(t *testing.T, pool *pgxpool.Pool, total int) *customSortFixture {
	t.Helper()
	ctx := context.Background()
	f := &customSortFixture{org: uuid.New(), user: uuid.New(), total: total}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	tag := f.org.String()[:8]
	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Sort', 'Live', $2, 'x')`, f.user, "i605-"+tag+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Issue 605', $2, $3)`, f.org, "i605-"+tag, f.user)
	exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
	      VALUES ($1, $2, 'owner', NOW())`, f.org, f.user)

	// Every third row lacks the field, every seventh carries it blank, and the
	// rest share four values so the id tiebreak carries most of the order.
	exec(`
		INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields, updated_at, created_at)
		SELECT gen_random_uuid(), $1, $2,
		       'i605-' || i || '-' || $3 || '@test.local',
		       'Lead' || i, 'Batch', '', '',
		       CASE
		           WHEN i % 3 = 0 THEN '{"Other Field": "x"}'::jsonb
		           WHEN i % 7 = 0 THEN '{"Industry": ""}'::jsonb
		           ELSE jsonb_build_object('Industry', (ARRAY['Fintech','Health','Retail','SaaS'])[(i % 4) + 1])
		       END,
		       NOW(), NOW() - (i || ' seconds')::interval
		FROM generate_series(1, $4) AS i`,
		f.user, f.org, tag, total)

	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM contacts WHERE organization_id = $1`, f.org},
			{`DELETE FROM organization_members WHERE organization_id = $1`, f.org},
			{`DELETE FROM organizations WHERE id = $1`, f.org},
			{`DELETE FROM users WHERE id = $1`, f.user},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

func TestLiveContactCustomFieldSortPagesEveryRow(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newCustomSortFixture(t, pool, 131)
	repo := NewContactRepostory(handle)
	ctx := context.Background()

	for _, reverse := range []bool{false, true} {
		filters := models.SearchContacts{SortBy: "custom:Industry", Reverse: reverse}

		whole, xerr := repo.Search(ctx, f.org.String(), nil, nil, filters, int32(f.total))
		if xerr != nil {
			t.Fatalf("reverse=%v single page: %v", reverse, xerr)
		}
		if len(whole.Data) != f.total {
			t.Fatalf("reverse=%v single page returned %d rows, want %d", reverse, len(whole.Data), f.total)
		}

		// The order the list shows: values sorted as text, with rows that lack
		// the field or carry it blank in one block, first when descending and
		// last when ascending.
		want := make([]uuid.UUID, 0, f.total)
		var prev string
		sawValue := false
		for i, c := range whole.Data {
			want = append(want, c.ID)
			v := c.CustomFields["Industry"]
			missing := v == ""
			switch {
			case !reverse && !missing && i > 0 && whole.Data[i-1].CustomFields["Industry"] == "":
				// DESC: once a value appears, no blank may follow it.
			case reverse && missing:
				sawValue = sawValue || false
			}
			if !missing {
				if sawValue && ((reverse && v < prev) || (!reverse && v > prev)) {
					t.Fatalf("reverse=%v values out of order at %d: %q after %q", reverse, i, v, prev)
				}
				prev = v
				sawValue = true
			}
		}
		firstMissing := whole.Data[0].CustomFields["Industry"] == ""
		lastMissing := whole.Data[f.total-1].CustomFields["Industry"] == ""
		if !reverse && !firstMissing {
			t.Fatalf("descending should put the rows without the field first")
		}
		if reverse && !lastMissing {
			t.Fatalf("ascending should put the rows without the field last")
		}
		if !reverse && lastMissing {
			t.Fatalf("descending should end on a row that has the field")
		}
		if reverse && firstMissing {
			t.Fatalf("ascending should start on a row that has the field")
		}

		seen, _ := pageThrough(t, repo, f.org.String(), filters, 10)
		assertExactlyOnce(t, seen, f.total)
		for i := range want {
			if i < len(seen) && seen[i] != want[i] {
				t.Fatalf("reverse=%v paged order diverges from the whole-list order at %d", reverse, i)
			}
		}

		ids, xerr := repo.SearchIDs(ctx, f.org.String(), filters, f.total)
		if xerr != nil {
			t.Fatalf("reverse=%v SearchIDs: %v", reverse, xerr)
		}
		if len(ids) != f.total {
			t.Fatalf("reverse=%v SearchIDs returned %d ids, want %d", reverse, len(ids), f.total)
		}
		for i := range want {
			if ids[i] != want[i] {
				t.Fatalf("reverse=%v SearchIDs order diverges from Search at %d", reverse, i)
			}
		}
	}
}

// A custom-field sort combined with a filter on the same field still pages
// every match once, and a sort on a field no row carries is one NULL block.
func TestLiveContactCustomFieldSortWithFilters(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newCustomSortFixture(t, pool, 60)
	repo := NewContactRepostory(handle)

	filtered := models.SearchContacts{
		SortBy: "custom:Industry",
		CustomFieldFilters: []models.SearchContactsFilter{
			{Name: "Industry", Value: "a", Type: models.SearchContactsFilterTypeContains},
		},
	}
	seen, _ := pageThrough(t, repo, f.org.String(), filtered, 7)
	want := 0
	for i := 1; i <= f.total; i++ {
		if i%3 == 0 || i%7 == 0 {
			continue
		}
		switch []string{"Fintech", "Health", "Retail", "SaaS"}[i%4] {
		case "Health", "Retail", "SaaS":
			want++
		}
	}
	assertExactlyOnce(t, seen, want)

	nobody, _ := pageThrough(t, repo, f.org.String(), models.SearchContacts{SortBy: "custom:Nobody Has This", Reverse: true}, 9)
	assertExactlyOnce(t, nobody, f.total)
}
