package repository

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/utils/paging"
)

// Issue #639: the admin lists paged on id < cursor while ordering by another
// column, so with random ids page two was an arbitrary slice. Each list is
// walked one row at a time here and must return every row exactly once.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/<db>?sslmode=disable \
//	  go test ./internal/repository/ -run LiveAdminPaging -v

// walk pages a list to the end, one row per page, the way the panel does.
func walk(t *testing.T, fetch func(cursor string) ([]uuid.UUID, models.Pagination)) []uuid.UUID {
	t.Helper()
	var seen []uuid.UUID
	cursor := ""
	for page := 0; page < 20; page++ {
		ids, p := fetch(cursor)
		seen = append(seen, ids...)
		if !p.HasMore {
			if p.NextCursor != nil {
				t.Fatalf("last page still carries next_cursor %q", *p.NextCursor)
			}
			return seen
		}
		if p.NextCursor == nil {
			t.Fatal("has_more with no next_cursor")
		}
		cursor = *p.NextCursor
	}
	t.Fatal("list never ended")
	return nil
}

func offsetOf(t *testing.T, cursor string) int {
	t.Helper()
	offset, xerr := paging.DecodeOffsetCursor(cursor)
	if xerr != nil {
		t.Fatalf("next_cursor %q does not decode as an offset: %v", cursor, xerr)
	}
	return offset
}

func sameSet(t *testing.T, what string, got, want []uuid.UUID) {
	t.Helper()
	g := append([]uuid.UUID(nil), got...)
	w := append([]uuid.UUID(nil), want...)
	sort.Slice(g, func(i, j int) bool { return g[i].String() < g[j].String() })
	sort.Slice(w, func(i, j int) bool { return w[i].String() < w[j].String() })
	if len(g) != len(w) {
		t.Fatalf("%s: walked %d rows %v, want %d %v", what, len(g), g, len(w), w)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("%s: walked %v, want %v", what, g, w)
		}
	}
}

func TestLiveAdminPagingWalksEveryRowOnce(t *testing.T) {
	_, pool := liveContactDB(t)
	ctx := context.Background()
	f := newAdminFixture(t, pool)
	admin := NewAdminRepository(pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}

	users := []uuid.UUID{f.user}
	mailboxes := []uuid.UUID{f.mailbox}
	campaigns := []uuid.UUID{f.campaign}
	for i := 0; i < 3; i++ {
		u, mb, c := uuid.New(), uuid.New(), uuid.New()
		exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
		      VALUES ($1, 'Paging', 'Live', $2, 'x')`, u, f.tag+"-u"+u.String()[:6]+"@test.local")
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, u) })
		exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html, provider)
		      VALUES ($1, $2, $3, $4, 'Paging', '', '', 'smtp_imap')`, mb, f.user, f.org, f.tag+"-mb"+mb.String()[:6]+"@test.local")
		exec(`INSERT INTO campaigns (id, user_id, organization_id, name, description, days, status, updated_at, created_at)
		      VALUES ($1, $2, $3, 'Paging', '', 62, 'active', NOW(), NOW())`, c, f.user, f.org)
		users = append(users, u)
		mailboxes = append(mailboxes, mb)
		campaigns = append(campaigns, c)
	}

	// One instant for every audit row, so only the id tiebreak orders them.
	at := time.Now().UTC().Truncate(time.Second)
	var audits []uuid.UUID
	for i := 0; i < 4; i++ {
		id := uuid.New()
		exec(`INSERT INTO admin_audit_logs (id, admin_user_id, action, target_type, target_id, created_at)
		      VALUES ($1, $2, 'paging', 'user', $2, $3)`, id, f.user, at)
		audits = append(audits, id)
	}

	t.Run("users", func(t *testing.T) {
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchUsers(ctx, &models.AdminUserSearch{Query: f.tag, Status: "all", Limit: 1, Offset: offsetOf(t, cursor)})
			if err != nil {
				t.Fatalf("SearchUsers: %v", err)
			}
			var ids []uuid.UUID
			for _, u := range res.Data {
				ids = append(ids, u.ID)
			}
			return ids, res.Pagination
		})
		sameSet(t, "users", got, users)
	})

	// Every explorer's "before" bound adds a day to a bare parameter, which
	// Postgres types as an interval unless the query casts it.
	t.Run("before and after date filters", func(t *testing.T) {
		today := at.Truncate(24 * time.Hour)
		org := &models.ParamUUID{UUID: f.org}
		res, err := admin.SearchUsers(ctx, &models.AdminUserSearch{Query: f.tag, Status: "all", CreatedAfter: &today, CreatedBefore: &today, Limit: 50})
		if err != nil {
			t.Fatalf("SearchUsers: %v", err)
		}
		if len(res.Data) != len(users) {
			t.Fatalf("users created today: %d, want %d", len(res.Data), len(users))
		}
		if _, err := admin.SearchCampaigns(ctx, &models.AdminCampaignSearch{OrgID: org, CreatedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("SearchCampaigns: %v", err)
		}
		if _, err := admin.SearchMailboxesForAdmin(ctx, &models.AdminMailboxSearch{OrgID: org, CreatedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("SearchMailboxesForAdmin: %v", err)
		}
		orgs := NewOrganizationRepository(pool)
		if _, err := orgs.SearchOrganizationsForAdmin(ctx, &models.AdminOrgSearch{CreatedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("SearchOrganizationsForAdmin: %v", err)
		}
		if _, err := orgs.ListLimitRequestsForAdmin(ctx, &models.AdminLimitRequestSearch{SubmittedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("ListLimitRequestsForAdmin: %v", err)
		}
		if _, err := NewAdminOutreachRepository(pool).Search(ctx, &models.AdminOutreachSearch{CreatedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("outreach Search: %v", err)
		}
		if _, err := NewDiscountCodeRepository(pool).List(ctx, &models.AdminDiscountSearch{CreatedBefore: &today, Limit: 1}); err != nil {
			t.Fatalf("discount List: %v", err)
		}
	})

	t.Run("users sorted by name", func(t *testing.T) {
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchUsers(ctx, &models.AdminUserSearch{Query: f.tag, Status: "all", SortBy: "name", SortDesc: true, Limit: 1, Offset: offsetOf(t, cursor)})
			if err != nil {
				t.Fatalf("SearchUsers: %v", err)
			}
			var ids []uuid.UUID
			for _, u := range res.Data {
				ids = append(ids, u.ID)
			}
			return ids, res.Pagination
		})
		sameSet(t, "users by name", got, users)
	})

	t.Run("users oldest first", func(t *testing.T) {
		var order []uuid.UUID
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchUsers(ctx, &models.AdminUserSearch{Query: f.tag, Status: "all", SortBy: "created_at", Limit: 1, Offset: offsetOf(t, cursor)})
			if err != nil {
				t.Fatalf("SearchUsers: %v", err)
			}
			for _, u := range res.Data {
				order = append(order, u.ID)
			}
			return order[len(order)-len(res.Data):], res.Pagination
		})
		sameSet(t, "users oldest first", got, users)
		if order[0] != f.user {
			t.Fatalf("ascending created_at starts at %s, want the fixture's first user %s", order[0], f.user)
		}
	})

	t.Run("campaigns", func(t *testing.T) {
		org := &models.ParamUUID{UUID: f.org}
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchCampaigns(ctx, &models.AdminCampaignSearch{OrgID: org, SortBy: "name", Limit: 1, Offset: offsetOf(t, cursor)})
			if err != nil {
				t.Fatalf("SearchCampaigns: %v", err)
			}
			var ids []uuid.UUID
			for _, c := range res.Data {
				ids = append(ids, c.ID)
			}
			return ids, res.Pagination
		})
		sameSet(t, "campaigns", got, campaigns)
	})

	t.Run("mailboxes", func(t *testing.T) {
		org := &models.ParamUUID{UUID: f.org}
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchMailboxesForAdmin(ctx, &models.AdminMailboxSearch{OrgID: org, Status: "all", SortBy: "last_synced_at", Limit: 1, Offset: offsetOf(t, cursor)})
			if err != nil {
				t.Fatalf("SearchMailboxesForAdmin: %v", err)
			}
			var ids []uuid.UUID
			for _, m := range res.Data {
				ids = append(ids, m.ID)
			}
			return ids, res.Pagination
		})
		sameSet(t, "mailboxes", got, mailboxes)
	})

	t.Run("user mailboxes", func(t *testing.T) {
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			emails, p, err := admin.GetUserEmails(ctx, f.user, offsetOf(t, cursor), 1)
			if err != nil {
				t.Fatalf("GetUserEmails: %v", err)
			}
			var ids []uuid.UUID
			for _, e := range emails {
				ids = append(ids, e.ID)
			}
			return ids, *p
		})
		sameSet(t, "user mailboxes", got, mailboxes)
	})

	t.Run("audit log", func(t *testing.T) {
		actor := &models.ParamUUID{UUID: f.user}
		day := at.Truncate(24 * time.Hour)
		got := walk(t, func(cursor string) ([]uuid.UUID, models.Pagination) {
			res, err := admin.SearchAuditLogs(ctx, &models.AdminAuditLogSearch{AdminUserID: actor, StartDate: &day, EndDate: &day, Cursor: cursor, Limit: 1})
			if err != nil {
				t.Fatalf("SearchAuditLogs: %v", err)
			}
			var ids []uuid.UUID
			for _, l := range res.Data {
				ids = append(ids, l.ID)
			}
			return ids, res.Pagination
		})
		// An end date names the whole day, so rows written today are inside it.
		sameSet(t, "audit log", got, audits)
	})

	t.Run("organizations, limit requests and outreach page past the first", func(t *testing.T) {
		orgs := NewOrganizationRepository(pool)
		if _, err := orgs.SearchOrganizationsForAdmin(ctx, &models.AdminOrgSearch{Status: "all", Limit: 1, Offset: 1}); err != nil {
			t.Fatalf("SearchOrganizationsForAdmin: %v", err)
		}
		if _, err := orgs.ListLimitRequestsForAdmin(ctx, &models.AdminLimitRequestSearch{Status: "all", Limit: 1, Offset: 1}); err != nil {
			t.Fatalf("ListLimitRequestsForAdmin: %v", err)
		}
		if _, err := NewAdminOutreachRepository(pool).Search(ctx, &models.AdminOutreachSearch{Limit: 1, Offset: 1}); err != nil {
			t.Fatalf("outreach Search: %v", err)
		}
		for name, call := range map[string]func() error{
			"ListAdmins":          func() error { _, err := admin.ListAdmins(ctx, 1, 1); return err },
			"ListWorkers":         func() error { _, err := admin.ListWorkers(ctx, 1, 1); return err },
			"GetPoolParticipants": func() error { _, err := admin.GetPoolParticipants(ctx, "free", 1, 1); return err },
			"ListBlockedAccounts": func() error { _, err := admin.ListBlockedAccounts(ctx, 1, 1); return err },
			"ListAppeals":         func() error { _, err := admin.ListAppeals(ctx, "", 1, 1); return err },
		} {
			if err := call(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	})
}
