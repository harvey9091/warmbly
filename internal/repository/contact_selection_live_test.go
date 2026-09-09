package repository

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/utils/paging"
)

// SearchIDs backs the dashboard's "select all matching" bulk actions, so it has
// to resolve exactly the set Search pages through. The two share a WHERE
// builder; this pins that they stay in step for every filter shape the leads
// and segment views send.
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveContactSearchIDs -v

type selectionFixture struct {
	org      uuid.UUID
	owner    uuid.UUID
	campaign uuid.UUID
}

func newSelectionFixture(t *testing.T, pool *pgxpool.Pool) *selectionFixture {
	t.Helper()
	ctx := context.Background()
	f := &selectionFixture{org: uuid.New(), owner: uuid.New(), campaign: uuid.New()}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	tag := f.org.String()[:8]

	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Sel', 'Live', $2, 'x')`, f.owner, "sel-"+tag+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Selection', $2, $3)`, f.org, "sel-"+tag, f.owner)
	exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
	      VALUES ($1, $2, 'owner', NOW())`, f.org, f.owner)
	exec(`INSERT INTO campaigns (id, user_id, organization_id, name, description, days, updated_at, created_at)
	      VALUES ($1, $2, $3, 'Selection campaign', '', 62, NOW(), NOW())`, f.campaign, f.owner, f.org)

	// 24 contacts: every third is unsubscribed, every fourth works at Acme,
	// and the first ten are leads on the campaign.
	for i := range 24 {
		id := uuid.New()
		company := "Globex"
		if i%4 == 0 {
			company = "Acme Freight"
		}
		exec(`INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields, subscribed, updated_at, created_at)
		      VALUES ($1, $2, $3, $4, $5, 'Live', $6, '', '{}'::jsonb, $7, NOW(), NOW() - make_interval(mins => $8))`,
			id, f.owner, f.org, fmt.Sprintf("sel-%s-%02d@test.local", tag, i),
			fmt.Sprintf("Person%02d", i), company, i%3 != 0, i)
		if i < 10 {
			exec(`INSERT INTO campaign_leads (campaign_id, contact_id) VALUES ($1, $2)`, f.campaign, id)
		}
	}

	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM campaign_leads WHERE campaign_id IN (SELECT id FROM campaigns WHERE organization_id = $1)`, f.org},
			{`DELETE FROM campaigns WHERE organization_id = $1`, f.org},
			{`DELETE FROM contacts WHERE organization_id = $1`, f.org},
			{`DELETE FROM organization_members WHERE organization_id = $1`, f.org},
			{`DELETE FROM organizations WHERE id = $1`, f.org},
			{`DELETE FROM users WHERE id = $1`, f.owner},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

func TestLiveContactSearchIDsMatchesSearch(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newSelectionFixture(t, pool)
	repo := NewContactRepostory(handle)
	ctx := context.Background()
	org := f.org.String()

	unsubscribed := false
	cases := []struct {
		name    string
		filters models.SearchContacts
		want    int
	}{
		{"whole workspace", models.SearchContacts{}, 24},
		{"text search", models.SearchContacts{Query: "Acme"}, 6},
		{"unsubscribed", models.SearchContacts{Subscribed: &unsubscribed}, 8},
		{"campaign leads", models.SearchContacts{CampaignIDs: []string{f.campaign.String()}}, 10},
		{"campaign leads, sorted by email", models.SearchContacts{
			CampaignIDs: []string{f.campaign.String()}, SortBy: "email", Reverse: true,
		}, 10},
		{"text search inside a campaign", models.SearchContacts{
			CampaignIDs: []string{f.campaign.String()}, Query: "Acme",
		}, 3},
		// campaign_count is the joined aggregate rather than a column, so the
		// keyset boundary has its own shape.
		{"sorted by campaign count", models.SearchContacts{SortBy: "campaign_count"}, 24},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Page through Search the way the table does, five rows at a time.
			var walked []uuid.UUID
			var cursor *paging.SortCursor
			for {
				page, xerr := repo.Search(ctx, org, nil, cursor, tc.filters, 5)
				if xerr != nil {
					t.Fatalf("search: %v", xerr)
				}
				for _, c := range page.Data {
					walked = append(walked, c.ID)
				}
				if !page.Pagination.HasMore || page.Pagination.NextCursor == nil {
					break
				}
				next, derr := paging.DecodeSortCursor(*page.Pagination.NextCursor)
				if derr != nil {
					t.Fatalf("decode cursor: %v", derr)
				}
				cursor = next
			}

			ids, xerr := repo.SearchIDs(ctx, org, tc.filters, models.MaxContactBulkSelection)
			if xerr != nil {
				t.Fatalf("search ids: %v", xerr)
			}
			if len(ids) != tc.want || len(walked) != tc.want {
				t.Fatalf("ids = %d, paged search = %d, want %d", len(ids), len(walked), tc.want)
			}
			if !sameIDSet(ids, walked) {
				t.Fatalf("SearchIDs resolved a different set than the list showed")
			}
			// The page walk is also the keyset regression: a boundary that
			// compares a row against itself repeats and skips rows instead.
			seen := make(map[uuid.UUID]struct{}, len(walked))
			for _, id := range walked {
				if _, dup := seen[id]; dup {
					t.Fatalf("paging returned %s twice", id)
				}
				seen[id] = struct{}{}
			}
		})
	}

	// The cap is reported by overshooting it: the caller refuses rather than
	// acting on a truncated selection.
	capped, xerr := repo.SearchIDs(ctx, org, models.SearchContacts{}, 5)
	if xerr != nil {
		t.Fatalf("capped search ids: %v", xerr)
	}
	if len(capped) != 6 {
		t.Fatalf("capped SearchIDs returned %d rows, want max+1 = 6", len(capped))
	}
}

func sameIDSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]uuid.UUID(nil), a...)
	bs := append([]uuid.UUID(nil), b...)
	less := func(s []uuid.UUID) func(i, j int) bool {
		return func(i, j int) bool { return s[i].String() < s[j].String() }
	}
	sort.Slice(as, less(as))
	sort.Slice(bs, less(bs))
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}
