package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// openHoursTimezone is a zone whose 8am-8pm mailbox band is open now and for
// the next ten minutes, so the pass is decided by the min-gap alone.
func openHoursTimezone(t *testing.T) string {
	t.Helper()
	open := func(at time.Time) bool { return at.Hour() >= 8 && at.Hour() < 20 }
	for _, name := range []string{"UTC", "America/New_York", "America/Los_Angeles", "Asia/Tokyo",
		"Europe/Berlin", "Asia/Dubai", "Pacific/Auckland", "Pacific/Honolulu"} {
		l, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		now := time.Now().In(l)
		if open(now) && open(now.Add(10*time.Minute)) {
			return name
		}
	}
	t.Fatal("no timezone in the spread stays inside 8am-8pm for the next ten minutes")
	return ""
}

// TestLiveWarmupOnOneMailboxDoesNotHoldTheCampaign: a new lead goes to a
// mailbox whose min-gap has elapsed, not to the one that just sent warmup.
func TestLiveWarmupOnOneMailboxDoesNotHoldTheCampaign(t *testing.T) {
	handle, pool := liveDB(t)
	tz := openHoursTimezone(t)
	f := newLiveFixture(t, pool, tz)
	ctx := context.Background()

	second := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO email_accounts (id, user_id, organization_id, email, name,
	          signature_plain, signature_html, provider, status, campaign_limit, min_wait_time, timezone)
	      VALUES ($1, $2, $3, $4, 'Second', '', '', 'smtp_imap', 'active', 50, 600, $5)`,
		second, f.user, f.org, "second-"+second.String()[:8]+"@test.local", tz); err != nil {
		t.Fatalf("add mailbox: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM tasks WHERE email_account_id = $1`, second)
		_, _ = pool.Exec(c, `DELETE FROM email_accounts WHERE id = $1`, second)
	})

	// Rotation breaks the tie by the lower id, so that is the mailbox the
	// warmup send lands on: the one rotation would otherwise pick.
	busy, free := f.mailbox, second
	if second.String() < f.mailbox.String() {
		busy, free = second, f.mailbox
	}
	sentAt := time.Now().Add(-time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, task_type, email_account_id, status, message_id, scheduled_at, completed_at, created_at, updated_at)
		VALUES ($1, 'warmup', $2, 'completed', '<warmup@test.local>', $3, $3, $3, $3)`, uuid.New(), busy, sentAt); err != nil {
		t.Fatalf("warmup send: %v", err)
	}

	at, pair, accountID, err := liveScheduler(t, handle, pool).CalculateNextCampaignTime(ctx, f.campaign)
	if err != nil {
		t.Fatalf("a free mailbox was in the pool, but the pass returned %v", err)
	}
	if pair == nil || accountID != free {
		t.Fatalf("pair=%v account=%s, want a send from the mailbox that did not just send (%s)", pair, accountID, free)
	}
	assertFuture(t, at)
}

// TestLiveBusyMatchingMailboxFallsBackUnderPrefer: under ESP "prefer", a
// matching mailbox inside its gap yields to a clear non-matching one.
func TestLiveBusyMatchingMailboxFallsBackUnderPrefer(t *testing.T) {
	handle, pool := liveDB(t)
	tz := openHoursTimezone(t)
	f := newLiveFixture(t, pool, tz)
	ctx := context.Background()

	matching := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO email_accounts (id, user_id, organization_id, email, name,
	          signature_plain, signature_html, provider, status, campaign_limit, min_wait_time, timezone)
	      VALUES ($1, $2, $3, $4, 'Gmail', '', '', 'gmail', 'active', 50, 600, $5)`,
		matching, f.user, f.org, "gmail-"+matching.String()[:8]+"@test.local", tz); err != nil {
		t.Fatalf("add mailbox: %v", err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM tasks WHERE email_account_id = $1`, matching)
		_, _ = pool.Exec(c, `DELETE FROM email_accounts WHERE id = $1`, matching)
	})
	// smtp_imap counts as a match under prefer, so the clear mailbox is Outlook.
	if _, err := pool.Exec(ctx, `UPDATE email_accounts SET provider = 'outlook' WHERE id = $1`, f.mailbox); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	for _, sql := range []string{
		`UPDATE campaigns SET esp_match_mode = 'prefer' WHERE id = $1`,
		`UPDATE contacts SET email = 'lead-' || left(id::text, 8) || '@gmail.com'
		 WHERE id IN (SELECT contact_id FROM campaign_leads WHERE campaign_id = $1)`,
	} {
		if _, err := pool.Exec(ctx, sql, f.campaign); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	sentAt := time.Now().Add(-time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, task_type, email_account_id, status, message_id, scheduled_at, completed_at, created_at, updated_at)
		VALUES ($1, 'warmup', $2, 'completed', '<warmup@test.local>', $3, $3, $3, $3)`, uuid.New(), matching, sentAt); err != nil {
		t.Fatalf("warmup send: %v", err)
	}

	_, pair, accountID, err := liveScheduler(t, handle, pool).CalculateNextCampaignTime(ctx, f.campaign)
	if err != nil {
		t.Fatalf("a clear mailbox was in the pool, but the pass returned %v", err)
	}
	if pair == nil || accountID != f.mailbox {
		t.Fatalf("pair=%v account=%s, want the clear non-matching mailbox %s", pair, accountID, f.mailbox)
	}
}
