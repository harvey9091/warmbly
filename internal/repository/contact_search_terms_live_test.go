package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/models"
)

// Contact search matches every word of the query against some field, so a
// full name finds the person whose first and last name are separate columns.
// Before issue #413 the query went at each column whole and "Test Demo"
// matched nothing while "Test" matched.
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveContactSearchTerms -v

type searchTermsFixture struct {
	org   uuid.UUID
	owner uuid.UUID
}

func newSearchTermsFixture(t *testing.T, pool *pgxpool.Pool) *searchTermsFixture {
	t.Helper()
	ctx := context.Background()
	f := &searchTermsFixture{org: uuid.New(), owner: uuid.New()}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	tag := f.org.String()[:8]

	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Terms', 'Live', $2, 'x')`, f.owner, "owner-"+tag+"@fixture.invalid")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Terms', $2, $3)`, f.org, "terms-"+tag, f.owner)
	exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
	      VALUES ($1, $2, 'owner', NOW())`, f.org, f.owner)

	people := []struct{ first, last, company string }{
		{"Test", "Demo", "Acme Freight"},
		{"Test", "Other", "Globex"},
		{"Demo", "Person", "Acme Freight"},
	}
	for i, p := range people {
		exec(`INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields, subscribed, updated_at, created_at)
		      VALUES ($1, $2, $3, $4, $5, $6, $7, '', '{}'::jsonb, true, NOW(), NOW())`,
			uuid.New(), f.owner, f.org,
			fmt.Sprintf("person-%s-%02d@fixture.invalid", tag, i), p.first, p.last, p.company)
	}

	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
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

func TestLiveContactSearchTerms(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newSearchTermsFixture(t, pool)
	repo := NewContactRepostory(handle)
	ctx := context.Background()
	org := f.org.String()

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"first name alone", "Test", 2},
		{"last name alone", "Demo", 2},
		{"full name", "Test Demo", 1},
		{"full name, reversed", "Demo Test", 1},
		{"full name, odd spacing", "  Test   Demo  ", 1},
		{"name and company", "Demo Acme", 2},
		{"a word that matches nobody", "Test Nobody", 0},
		{"empty query lists everyone", "", 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, xerr := repo.Search(ctx, org, nil, nil, models.SearchContacts{Query: tc.query}, 50)
			if xerr != nil {
				t.Fatalf("search: %v", xerr)
			}
			if len(page.Data) != tc.want {
				t.Fatalf("search %q returned %d contacts, want %d", tc.query, len(page.Data), tc.want)
			}
			// The list and "select all matching" share a WHERE builder, so
			// the bulk selection has to resolve the same rows.
			ids, xerr := repo.SearchIDs(ctx, org, models.SearchContacts{Query: tc.query}, models.MaxContactBulkSelection)
			if xerr != nil {
				t.Fatalf("search ids: %v", xerr)
			}
			if len(ids) != tc.want {
				t.Fatalf("SearchIDs %q returned %d contacts, want %d", tc.query, len(ids), tc.want)
			}
		})
	}
}

func TestContactSearchTermsSplit(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"", nil},
		{"   ", nil},
		{"Test", []string{"Test"}},
		{"  Test   Demo ", []string{"Test", "Demo"}},
		{"a b c d e f g h", []string{"a", "b", "c", "d", "e", "f"}},
	}
	for _, tc := range cases {
		got := contactSearchTerms(tc.query)
		if len(got) != len(tc.want) {
			t.Fatalf("contactSearchTerms(%q) = %v, want %v", tc.query, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("contactSearchTerms(%q) = %v, want %v", tc.query, got, tc.want)
			}
		}
	}
}
