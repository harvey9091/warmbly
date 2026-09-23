package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// A mailbox with no timezone of its own reads its hours in the workspace's.
// Every read path that hands a mailbox to a scheduler or the consumer carries
// the workspace zone, and a campaign created without a zone is created in it.
//
// Run against a migrated database:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/<db>?sslmode=disable \
//	  go test ./internal/repository/ -run LiveMailboxClockTimezone -v
func TestLiveMailboxClockTimezoneFollowsTheWorkspace(t *testing.T) {
	handle, pool := liveContactDB(t)
	requireSchemaVersion(t, pool, 198)
	ctx := context.Background()
	f := newAdminFixture(t, pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	exec(`UPDATE organizations SET timezone = 'America/New_York' WHERE id = $1`, f.org)
	exec(`UPDATE email_accounts SET timezone = '', warmup_tag = $2 WHERE id = $1`, f.mailbox, f.tag)
	exec(`INSERT INTO campaign_senders (campaign_id, email_account_id) VALUES ($1, $2)`, f.campaign, f.mailbox)
	tagID := uuid.New()
	exec(`INSERT INTO tags (id, organization_id, title, color, position) VALUES ($1, $2, $3, '#336699', 0)`, tagID, f.org, f.tag)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM tags WHERE id = $1`, tagID) })
	exec(`INSERT INTO email_tags (email_id, tag_id) VALUES ($1, $2)`, f.mailbox, tagID)

	emails := NewEmailRepostory(handle, nil)
	scope := AccountScope{OrgID: &f.org}
	want := func(what string, e *models.Email) {
		t.Helper()
		if e == nil {
			t.Fatalf("%s: mailbox not returned", what)
		}
		if e.Timezone != "" || e.OrgTimezone != "America/New_York" || e.ClockTimezone() != "America/New_York" {
			t.Fatalf("%s: timezone %q org %q clock %q, want the workspace zone", what, e.Timezone, e.OrgTimezone, e.ClockTimezone())
		}
	}

	byID, xerr := emails.GetByID(ctx, f.mailbox)
	if xerr != nil {
		t.Fatalf("GetByID: %v", xerr)
	}
	want("GetByID", byID)

	got, xerr := emails.Get(ctx, f.org.String(), f.mailbox.String())
	if xerr != nil {
		t.Fatalf("Get: %v", xerr)
	}
	want("Get", got)

	search, xerr := emails.Search(ctx, f.org.String(), "", nil, nil, 50, nil)
	if xerr != nil {
		t.Fatalf("Search: %v", xerr)
	}
	want("Search", pick(search.Data, f.mailbox))

	active, xerr := emails.GetAllActiveInScope(ctx, scope)
	if xerr != nil {
		t.Fatalf("GetAllActiveInScope: %v", xerr)
	}
	want("GetAllActiveInScope", pick(active, f.mailbox))

	tagged, xerr := emails.GetByTags(ctx, scope, []string{tagID.String()})
	if xerr != nil {
		t.Fatalf("GetByTags: %v", xerr)
	}
	want("GetByTags", pick(tagged, f.mailbox))

	senders, xerr := emails.GetByCampaignSenders(ctx, scope, f.campaign)
	if xerr != nil {
		t.Fatalf("GetByCampaignSenders: %v", xerr)
	}
	var sender *models.Email
	for i := range senders {
		if senders[i].Account.ID == f.mailbox {
			sender = &senders[i].Account
		}
	}
	want("GetByCampaignSenders", sender)

	// The mailbox's own zone wins over the workspace's once it has one.
	exec(`UPDATE email_accounts SET timezone = 'Europe/Paris' WHERE id = $1`, f.mailbox)
	own, xerr := emails.GetByID(ctx, f.mailbox)
	if xerr != nil {
		t.Fatalf("GetByID after own zone: %v", xerr)
	}
	if own.ClockTimezone() != "Europe/Paris" || own.OrgTimezone != "America/New_York" {
		t.Fatalf("own zone: clock %q org %q", own.ClockTimezone(), own.OrgTimezone)
	}

	// A campaign created without a timezone follows the workspace's.
	campaigns := NewCampaignRepostory(handle)
	created, xerr := campaigns.Create(ctx, f.user.String(), &f.org, &models.CreateCampaign{Name: f.tag + " default zone"})
	if xerr != nil {
		t.Fatalf("Create campaign: %v", xerr)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM campaigns WHERE id = $1`, created.ID) })
	if created.Timezone != "" || created.EffectiveTimezone != "America/New_York" || created.ClockTimezone() != "America/New_York" {
		t.Fatalf("campaign timezone %q effective %q, want to follow the workspace zone", created.Timezone, created.EffectiveTimezone)
	}
	reread, err := campaigns.GetByID(ctx, created.ID)
	if err != nil || reread.ClockTimezone() != "America/New_York" {
		t.Fatalf("GetByID: err %v, clock %q", err, reread.ClockTimezone())
	}
	exec(`UPDATE organizations SET timezone = '' WHERE id = $1`, f.org)
	bare, xerr := campaigns.Create(ctx, f.user.String(), &f.org, &models.CreateCampaign{Name: f.tag + " no zone"})
	if xerr != nil {
		t.Fatalf("Create campaign without a workspace zone: %v", xerr)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM campaigns WHERE id = $1`, bare.ID) })
	if bare.Timezone != "" || bare.ClockTimezone() != "UTC" {
		t.Fatalf("campaign timezone %q clock %q, want UTC when the workspace has none", bare.Timezone, bare.ClockTimezone())
	}
	paris := "Europe/Paris"
	pinned, xerr := campaigns.Create(ctx, f.user.String(), &f.org, &models.CreateCampaign{Name: f.tag + " own zone", Timezone: &paris})
	if xerr != nil {
		t.Fatalf("Create campaign with its own zone: %v", xerr)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM campaigns WHERE id = $1`, pinned.ID) })
	if pinned.ClockTimezone() != "Europe/Paris" {
		t.Fatalf("pinned zone: clock %q", pinned.ClockTimezone())
	}
}

func pick(list []models.Email, id uuid.UUID) *models.Email {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}
