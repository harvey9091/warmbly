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
