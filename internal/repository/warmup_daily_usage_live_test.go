package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// warmupUsageFixture is one mailbox in one workspace, plus a helper that
// writes rows directly so each test states exactly the ledger it is testing.
type warmupUsageFixture struct {
	user    uuid.UUID
	org     uuid.UUID
	account uuid.UUID
	exec    func(query string, args ...any)
}

func newWarmupUsageFixture(t *testing.T, pool *pgxpool.Pool) warmupUsageFixture {
	t.Helper()
	ctx := context.Background()
	f := warmupUsageFixture{user: uuid.New(), org: uuid.New(), account: uuid.New()}
	f.exec = func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	f.exec(`INSERT INTO users (id, first_name, last_name, email)
	        VALUES ($1, 'Warmup', 'Usage', $2)`, f.user, "warmup-usage-"+uuid.NewString()+"@example.test")
	f.exec(`INSERT INTO organizations (id, name, owner_user_id)
	        VALUES ($1, 'Warmup usage', $2)`, f.org, f.user)
	f.exec(`INSERT INTO email_accounts
	          (id, user_id, organization_id, email, name, signature_plain, signature_html, provider, warmup_max)
	        VALUES ($1, $2, $3, $4, 'Warmup usage', '', '', 'smtp_imap', 40)`,
		f.account, f.user, f.org, "mailbox-"+uuid.NewString()+"@example.test")
	t.Cleanup(func() {
		for _, step := range []struct {
			query string
			arg   uuid.UUID
		}{
			{`DELETE FROM email_accounts WHERE id = $1`, f.account},
			{`DELETE FROM organizations WHERE id = $1`, f.org},
			{`DELETE FROM users WHERE id = $1`, f.user},
		} {
			if _, err := pool.Exec(context.Background(), step.query, step.arg); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
	return f
}

// The reported number and the cap the scheduler enforces have to count the
// same thing. Reading warmup_statistics for it let a send the worker refused
// be reported forever while the cap (completed warmup tasks) had already freed
// the slot, which is how sent_today reached 21 against a target of 10 (#574).
func TestLiveAccountDailyUsageCountsWarmupTasksNotTheStatisticsRow(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	f := newWarmupUsageFixture(t, pool)
	day := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

	// Two dispatches: one the worker delivered, one it refused. The statistics
	// row was stamped at dispatch for both, before the worker had answered.
	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	        VALUES ($1, 'warmup', $2, 'completed', '', $3)`, uuid.New(), f.account, day)
	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	        VALUES ($1, 'warmup', $2, 'failed', '', $3)`, uuid.New(), f.account, day)
	f.exec(`INSERT INTO warmup_statistics (email_account_id, date, emails_sent, target_volume)
	        VALUES ($1, DATE($2), 2, 10)`, f.account, day)

	repo := &analyticsRepository{DB: handle}
	usage, xerr := repo.GetAccountDailyUsage(ctx, f.account, day)
	if xerr != nil {
		t.Fatalf("GetAccountDailyUsage: %v", xerr)
	}
	if usage.WarmupSent != 1 {
		t.Fatalf("warmup_sent = %d, want 1: only the delivered send counts, the way the cap counts it", usage.WarmupSent)
	}
	if usage.WarmupLimit != 40 {
		t.Fatalf("warmup_limit = %d, want 40", usage.WarmupLimit)
	}
}

// A campaign task must not be mistaken for a warmup one, and a send on another
// day must not leak into today.
func TestLiveAccountDailyUsageScopesWarmupToTypeAndDay(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	f := newWarmupUsageFixture(t, pool)
	day := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	        VALUES ($1, 'campaign', $2, 'completed', '<c@example.test>', $3)`, uuid.New(), f.account, day)
	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	        VALUES ($1, 'warmup', $2, 'completed', '', $3)`, uuid.New(), f.account, day.AddDate(0, 0, -1))

	repo := &analyticsRepository{DB: handle}
	usage, xerr := repo.GetAccountDailyUsage(ctx, f.account, day)
	if xerr != nil {
		t.Fatalf("GetAccountDailyUsage: %v", xerr)
	}
	if usage.WarmupSent != 0 {
		t.Fatalf("warmup_sent = %d, want 0", usage.WarmupSent)
	}
	if usage.CampaignSent != 1 {
		t.Fatalf("campaign_sent = %d, want 1", usage.CampaignSent)
	}
}

// The counters are taken at dispatch, so a send the worker could not deliver
// has to be given back, or the mailbox is reported as having sent it forever
// and its 7-day totals (the ramp's own denominator) drift up with every
// failure (#574).
func TestLiveGiveBackDailySendReversesADispatchedWarmupSend(t *testing.T) {
	_, pool := liveContactDB(t)
	ctx := context.Background()
	f := newWarmupUsageFixture(t, pool)
	day := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	repo := &warmupRepository{db: pool}

	newThread, reply := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{newThread, reply} {
		f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
		        VALUES ($1, 'warmup', $2, 'completed', '', $3)`, id, f.account, day)
	}
	// conversation_turn is what the send itself recorded: 0 opened a thread,
	// above 0 answered one, and only the second took the reply counter.
	f.exec(`INSERT INTO warmup_tokens (token, task_id, sender_account_id, recipient_account_id, conversation_turn)
	        VALUES ($1, $2, $3, $3, 0)`, uuid.New(), newThread, f.account)
	f.exec(`INSERT INTO warmup_tokens (token, task_id, sender_account_id, recipient_account_id, conversation_turn)
	        VALUES ($1, $2, $3, $3, 2)`, uuid.New(), reply, f.account)
	f.exec(`INSERT INTO warmup_statistics (email_account_id, date, emails_sent, emails_replied, target_volume)
	        VALUES ($1, DATE($2), 2, 1, 10)`, f.account, day)

	read := func() (sent, replied int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT emails_sent, emails_replied FROM warmup_statistics
		                              WHERE email_account_id = $1 AND date = DATE($2)`, f.account, day).Scan(&sent, &replied); err != nil {
			t.Fatalf("read statistics: %v", err)
		}
		return sent, replied
	}

	// The new thread: the send comes back, the reply count is untouched.
	if err := repo.GiveBackDailySend(ctx, f.account, newThread, day); err != nil {
		t.Fatalf("GiveBackDailySend: %v", err)
	}
	if sent, replied := read(); sent != 1 || replied != 1 {
		t.Fatalf("after giving back a new thread: sent=%d replied=%d, want 1/1", sent, replied)
	}

	// The reply: both come back, because both were taken at dispatch.
	if err := repo.GiveBackDailySend(ctx, f.account, reply, day); err != nil {
		t.Fatalf("GiveBackDailySend: %v", err)
	}
	if sent, replied := read(); sent != 0 || replied != 0 {
		t.Fatalf("after giving back a reply: sent=%d replied=%d, want 0/0", sent, replied)
	}

	// Floored: a duplicate result, or a give-back for a day whose row was
	// never incremented, must not drive the day negative.
	if err := repo.GiveBackDailySend(ctx, f.account, reply, day); err != nil {
		t.Fatalf("GiveBackDailySend (repeat): %v", err)
	}
	if sent, replied := read(); sent != 0 || replied != 0 {
		t.Fatalf("a repeated give-back went below zero: sent=%d replied=%d", sent, replied)
	}
}

// The give-back is scoped to the day the send was counted on, so a failure
// that arrives after midnight cannot take a send off the wrong day.
func TestLiveGiveBackDailySendLeavesOtherDaysAlone(t *testing.T) {
	_, pool := liveContactDB(t)
	ctx := context.Background()
	f := newWarmupUsageFixture(t, pool)
	counted := time.Date(2026, time.September, 16, 23, 50, 0, 0, time.UTC)
	next := counted.AddDate(0, 0, 1)
	repo := &warmupRepository{db: pool}

	taskID := uuid.New()
	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	        VALUES ($1, 'warmup', $2, 'completed', '', $3)`, taskID, f.account, counted)
	f.exec(`INSERT INTO warmup_statistics (email_account_id, date, emails_sent, target_volume)
	        VALUES ($1, DATE($2), 5, 10)`, f.account, counted)
	f.exec(`INSERT INTO warmup_statistics (email_account_id, date, emails_sent, target_volume)
	        VALUES ($1, DATE($2), 3, 10)`, f.account, next)

	if err := repo.GiveBackDailySend(ctx, f.account, taskID, counted); err != nil {
		t.Fatalf("GiveBackDailySend: %v", err)
	}
	for _, tc := range []struct {
		day  time.Time
		want int
	}{{counted, 4}, {next, 3}} {
		var sent int
		if err := pool.QueryRow(ctx, `SELECT emails_sent FROM warmup_statistics
		                              WHERE email_account_id = $1 AND date = DATE($2)`, f.account, tc.day).Scan(&sent); err != nil {
			t.Fatalf("read statistics: %v", err)
		}
		if sent != tc.want {
			t.Fatalf("%s emails_sent = %d, want %d", tc.day.Format("2006-01-02"), sent, tc.want)
		}
	}
}
