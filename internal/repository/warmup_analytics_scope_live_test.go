package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Workspace warmup analytics aggregate every mailbox into one row per date.
func TestLiveWarmupAnalyticsAreOrganizationScoped(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	userID, orgID, accountID, secondAccountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	day := time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC)

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	exec(`INSERT INTO users (id, first_name, last_name, email)
	      VALUES ($1, 'Warmup', 'Scope', $2)`, userID, "warmup-scope-"+uuid.NewString()+"@example.test")
	exec(`INSERT INTO organizations (id, name, owner_user_id)
	      VALUES ($1, 'Warmup scope', $2)`, orgID, userID)
	exec(`INSERT INTO email_accounts
	        (id, user_id, organization_id, email, name, signature_plain, signature_html, provider)
	      VALUES ($1, $2, $3, $4, 'Warmup scope', '', '', 'smtp_imap')`,
		accountID, userID, orgID, "warmup-scope-mailbox-"+uuid.NewString()+"@example.test")
	exec(`INSERT INTO email_accounts
	        (id, user_id, organization_id, email, name, signature_plain, signature_html, provider)
	      VALUES ($1, $2, $3, $4, 'Warmup scope second', '', '', 'smtp_imap')`,
		secondAccountID, userID, orgID, "warmup-scope-mailbox-"+uuid.NewString()+"@example.test")
	exec(`INSERT INTO warmup_statistics (email_account_id, date, emails_sent, emails_replied, target_volume)
	      VALUES ($1, $3, 8, 3, 10), ($2, $3, 5, 2, 8)`, accountID, secondAccountID, day)
	// Arrivals: two for the first mailbox on the planned day, one for the second
	// mailbox the day after, when nothing was planned.
	exec(`INSERT INTO warmup_received (email_account_id, internal_id, message_id, sender_account_id, created_at)
	      VALUES ($1, gen_random_uuid(), '<a@example.test>', $2, $3::timestamptz + interval '9 hours'),
	             ($1, gen_random_uuid(), '<b@example.test>', $2, $3::timestamptz + interval '15 hours'),
	             ($2, gen_random_uuid(), '<c@example.test>', $1, $3::timestamptz + interval '1 day 2 hours')`,
		accountID, secondAccountID, day)

	t.Cleanup(func() {
		for _, step := range []struct {
			query string
			arg   uuid.UUID
		}{
			{`DELETE FROM warmup_received WHERE email_account_id IN (SELECT id FROM email_accounts WHERE organization_id = $1)`, orgID},
			{`DELETE FROM email_accounts WHERE organization_id = $1`, orgID},
			{`DELETE FROM organizations WHERE id = $1`, orgID},
			{`DELETE FROM users WHERE id = $1`, userID},
		} {
			if _, err := pool.Exec(context.Background(), step.query, step.arg); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})

	repo := &analyticsRepository{DB: handle}
	stats, xerr := repo.GetWarmupStats(ctx, orgID, &accountID, day, day)
	if xerr != nil {
		t.Fatalf("GetWarmupStats: %v", xerr)
	}
	if len(stats) != 1 || stats[0].EmailsSent != 8 || stats[0].EmailsReplied != 3 || stats[0].EmailsReceived != 2 || !stats[0].Active {
		t.Fatalf("warmup stats = %+v, want the organization's 8 sends, 3 replies and 2 arrivals on an active day", stats)
	}
	stats, xerr = repo.GetWarmupStats(ctx, orgID, nil, day, day)
	if xerr != nil {
		t.Fatalf("GetWarmupStats workspace: %v", xerr)
	}
	if len(stats) != 1 || stats[0].EmailsSent != 13 || stats[0].EmailsReplied != 5 || stats[0].TargetVolume != 18 || stats[0].EmailsReceived != 2 {
		t.Fatalf("workspace warmup stats = %+v, want one date with 13 sends, 5 replies, target 18 and 2 arrivals", stats)
	}

	// The day after had arrivals and no plan: it lists as inactive with what came in.
	stats, xerr = repo.GetWarmupStats(ctx, orgID, nil, day, day.AddDate(0, 0, 1))
	if xerr != nil {
		t.Fatalf("GetWarmupStats two days: %v", xerr)
	}
	if len(stats) != 2 || stats[1].Date != "2026-09-16" || stats[1].EmailsReceived != 1 || stats[1].EmailsSent != 0 || stats[1].Active {
		t.Fatalf("two-day warmup stats = %+v, want a second, inactive day with one arrival", stats)
	}
}
