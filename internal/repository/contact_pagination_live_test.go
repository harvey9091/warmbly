package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/utils/paging"
)

// Regression cover for issue #382: a campaign's Leads tab stopped loading after
// a few pages, showing 198 of ~600 leads with no "Load more" left.
//
// The keyset cursor compared the sort column against a subquery that named the
// OUTER row's alias (`SELECT c.created_at FROM contacts WHERE id = $n`), which
// Postgres resolves as a correlated reference to the row being tested. The
// whole boundary collapsed to `c.id >= <cursor uuid>`, so each page returned
// the newest rows of a randomly shrinking id range: duplicates on screen and a
// list that ran out long before the leads did. The same loop backs ExportAll,
// so exports were truncated the same way.
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveContactPagination -v

// pagedOrgFixture is one organization whose single campaign holds every contact
// as a lead, sized past several pages.
type pagedOrgFixture struct {
	org      uuid.UUID
	user     uuid.UUID
	campaign uuid.UUID
	total    int
}

func newPagedOrgFixture(t *testing.T, pool *pgxpool.Pool, total int) *pagedOrgFixture {
	t.Helper()
	ctx := context.Background()
	f := &pagedOrgFixture{org: uuid.New(), user: uuid.New(), campaign: uuid.New(), total: total}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Paged', 'Live', $2, 'x')`, f.user, "i382-"+f.user.String()[:8]+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Issue 382', $2, $3)`, f.org, "i382-"+f.org.String()[:8], f.user)
	exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
	      VALUES ($1, $2, 'owner', NOW())`, f.org, f.user)
	exec(`INSERT INTO campaigns (id, user_id, organization_id, name, description, days, updated_at, created_at)
	      VALUES ($1, $2, $3, 'Q3 cold outreach', '', 62, NOW(), NOW())`, f.campaign, f.user, f.org)

	// A bulk import stamps a whole batch with the same created_at, so most of
	// the list is one big tie the id tiebreak has to carry. Names repeat for the
	// same reason; the sort must still be a total order.
	exec(`
		INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields, updated_at, created_at)
		SELECT gen_random_uuid(), $1, $2,
		       'i382-' || i || '-' || $3 || '@test.local',
		       'Lead' || (i % 7), 'Batch' || (i % 3), '', '', '{}'::jsonb,
		       NOW() - ((i % 5) || ' minutes')::interval,
		       NOW() - ((i / 100) || ' hours')::interval
		FROM generate_series(1, $4) AS i`,
		f.user, f.org, f.org.String()[:8], total)
	exec(`INSERT INTO campaign_leads (campaign_id, contact_id)
	      SELECT $1, id FROM contacts WHERE organization_id = $2`, f.campaign, f.org)

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
			{`DELETE FROM users WHERE id = $1`, f.user},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

// pageThrough walks the list the way "Load more" does and returns the ids in
// the order they were served, plus the page count.
func pageThrough(t *testing.T, repo ContactRepository, orgID string, filters models.SearchContacts, limit int32) ([]uuid.UUID, int) {
	t.Helper()
	ctx := context.Background()
	var (
		seen   []uuid.UUID
		cursor *paging.SortCursor
		pages  int
	)
	for {
		page, xerr := repo.Search(ctx, orgID, nil, cursor, filters, limit)
		if xerr != nil {
			t.Fatalf("page %d: %v", pages, xerr)
		}
		pages++
		for _, c := range page.Data {
			seen = append(seen, c.ID)
		}
		if !page.Pagination.HasMore {
			if page.Pagination.NextCursor != nil {
				t.Fatalf("page %d says no more but still handed out a cursor", pages)
			}
			return seen, pages
		}
		if page.Pagination.NextCursor == nil {
			t.Fatalf("page %d says there is more but handed out no cursor", pages)
		}
		next, xerr := paging.DecodeSortCursor(*page.Pagination.NextCursor)
		if xerr != nil {
			t.Fatalf("page %d cursor: %v", pages, xerr)
		}
		cursor = next
		if pages > 200 {
			t.Fatal("pagination did not terminate")
		}
	}
}

// The issue as reported: ~600 leads, 50 a page, and the list has to hand back
// every lead exactly once.
func TestLiveContactPaginationWalksEveryLead(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newPagedOrgFixture(t, pool, 617)
	repo := NewContactRepostory(handle)

	seen, pages := pageThrough(t, repo, f.org.String(), models.SearchContacts{
		CampaignIDs: []string{f.campaign.String()},
	}, 50)

	if pages != 13 {
		t.Errorf("walked %d pages, want 13 (617 leads at 50 a page)", pages)
	}
	assertExactlyOnce(t, seen, f.total)
}

// Every sort the dashboard offers, in both directions. campaign_count sorts on
// a computed column that WHERE cannot reach by its SELECT alias, so a cursor
// there used to be a hard SQL error rather than a short list.
func TestLiveContactPaginationCoversEverySort(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newPagedOrgFixture(t, pool, 137)
	repo := NewContactRepostory(handle)

	for _, sortBy := range []string{"", "first_name", "last_name", "email", "created_at", "updated_at", "campaign_count"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", sortBy, reverse), func(t *testing.T) {
				filters := models.SearchContacts{
					CampaignIDs: []string{f.campaign.String()},
					SortBy:      sortBy,
					Reverse:     reverse,
				}
				// One page holding everything is the order to check against.
				whole, xerr := repo.Search(context.Background(), f.org.String(), nil, nil, filters, int32(f.total))
				if xerr != nil {
					t.Fatalf("single page: %v", xerr)
				}
				want := make([]uuid.UUID, 0, f.total)
				for _, c := range whole.Data {
					want = append(want, c.ID)
				}

				seen, _ := pageThrough(t, repo, f.org.String(), filters, 10)
				assertExactlyOnce(t, seen, f.total)
				for i := range want {
					if i < len(seen) && seen[i] != want[i] {
						t.Fatalf("paged order diverges from the whole-list order at %d", i)
					}
				}
			})
		}
	}
}

// The org-wide contacts list (no campaign scope) pages the same way, and the
// export walks the same cursor loop, so it has to reach every row too.
func TestLiveContactPaginationExportsEveryRow(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newPagedOrgFixture(t, pool, 617)
	repo := NewContactRepostory(handle)

	seen, _ := pageThrough(t, repo, f.org.String(), models.SearchContacts{}, 50)
	assertExactlyOnce(t, seen, f.total)

	filters := models.SearchContacts{CampaignIDs: []string{f.campaign.String()}}
	rows, xerr := repo.ExportAll(context.Background(), f.org.String(), &filters, nil, 10000)
	if xerr != nil {
		t.Fatalf("export: %v", xerr)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, c := range rows {
		ids = append(ids, c.ID)
	}
	assertExactlyOnce(t, ids, f.total)
}

// The filters that append their own bound parameters sit either side of the
// cursor's in the argument list, and a segment condition compiles a whole SQL
// fragment of its own. Page through each of them so a shifted placeholder
// shows up as a wrong or missing row rather than in production.
func TestLiveContactPaginationWalksFilteredLists(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newPagedOrgFixture(t, pool, 137)
	repo := NewContactRepostory(handle)
	segments := NewSegmentRepository(handle)

	// Every lead is in exactly one campaign and every email carries the org tag,
	// so each of these scopes the list to the whole fixture.
	one := 1
	seg, xerr := segments.Create(context.Background(), f.org, &f.user, &models.Segment{
		Name:  "Everyone " + f.org.String()[:8],
		Color: "#0284c7",
		Match: models.SegmentMatchAll,
		Conditions: []models.SegmentCondition{
			{Field: "email", Operator: "contains", Value: "i382-"},
		},
	})
	if xerr != nil {
		t.Fatalf("create segment: %v", xerr)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM segments WHERE organization_id = $1`, f.org); err != nil {
			t.Errorf("cleanup segments: %v", err)
		}
	})

	for name, filters := range map[string]models.SearchContacts{
		"query":         {CampaignIDs: []string{f.campaign.String()}, Query: "i382-"},
		"min campaigns": {CampaignIDs: []string{f.campaign.String()}, MinCampaigns: &one},
		"max campaigns": {CampaignIDs: []string{f.campaign.String()}, MaxCampaigns: &one},
		"segment":       {SegmentIDs: []string{seg.ID.String()}},
		"segment sorted by campaign count": {
			SegmentIDs: []string{seg.ID.String()}, MinCampaigns: &one, SortBy: "campaign_count", Reverse: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			seen, _ := pageThrough(t, repo, f.org.String(), filters, 10)
			assertExactlyOnce(t, seen, f.total)
		})
	}
}

// A token minted under one ordering describes a position that does not exist
// under another, and a token from the old id-only format is not a position at
// all. Both are client contract errors, not silently wrong pages.
func TestLiveContactPaginationRejectsAForeignCursor(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newPagedOrgFixture(t, pool, 20)
	repo := NewContactRepostory(handle)
	ctx := context.Background()

	first, xerr := repo.Search(ctx, f.org.String(), nil, nil, models.SearchContacts{}, 5)
	if xerr != nil {
		t.Fatalf("first page: %v", xerr)
	}
	cursor, xerr := paging.DecodeSortCursor(*first.Pagination.NextCursor)
	if xerr != nil {
		t.Fatalf("decode: %v", xerr)
	}
	if _, xerr := repo.Search(ctx, f.org.String(), nil, cursor, models.SearchContacts{SortBy: "email"}, 5); xerr == nil {
		t.Fatal("a created_at cursor must not be accepted on an email sort")
	}
	if _, xerr := repo.Search(ctx, f.org.String(), nil, &paging.SortCursor{Sort: "created_at:desc", ID: cursor.ID}, models.SearchContacts{}, 5); xerr == nil {
		t.Fatal("a NULL boundary on a NOT NULL column must not be accepted")
	}

	// A boundary the cast would choke on is the client's mistake, so it has to
	// be refused before it reaches Postgres and comes back as a 500.
	for _, tc := range []struct {
		sort  string
		value string
		as    models.SearchContacts
	}{
		{"created_at:desc", "yesterday", models.SearchContacts{}},
		{"created_at:desc", "", models.SearchContacts{}},
		{"campaign_count:desc", "a lot", models.SearchContacts{SortBy: "campaign_count"}},
	} {
		bad := &paging.SortCursor{Sort: tc.sort, Value: &tc.value, ID: cursor.ID}
		_, xerr := repo.Search(ctx, f.org.String(), nil, bad, tc.as, 5)
		if xerr == nil {
			t.Fatalf("%q boundary %q must be refused", tc.sort, tc.value)
		}
		if xerr.Code != errx.BadRequest {
			t.Fatalf("%q boundary %q returned %v, want a bad request", tc.sort, tc.value, xerr.Code)
		}
	}
}

func assertExactlyOnce(t *testing.T, seen []uuid.UUID, total int) {
	t.Helper()
	unique := make(map[uuid.UUID]int, len(seen))
	for _, id := range seen {
		unique[id]++
	}
	dupes := 0
	for _, n := range unique {
		if n > 1 {
			dupes++
		}
	}
	if dupes > 0 {
		t.Errorf("%d contacts were served more than once", dupes)
	}
	if len(unique) != total {
		t.Errorf("saw %d distinct contacts (%d rows), want %d", len(unique), len(seen), total)
	}
}
