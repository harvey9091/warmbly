package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The day's plan reads the leads through the same routing the send path
// uses, and the per-campaign sender ledger through the same predicate the
// per-mailbox budget uses. Both are SQL, so both are checked live.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveLeadSupply -v

func TestLiveLeadSupplyCountsWhereEveryLeadStands(t *testing.T) {
	_, pool := liveContactDB(t)
	f := newRoutedPairsFixture(t, pool, 4)
	ctx := context.Background()
	repo := NewCampaignProgressRepository(pool)

	// Lead 1 has had the only step: their flow is over. Lead 2 is held with
	// no end. Leads 0 and 3 are due now.
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, sent_at)
		VALUES ($1, $2, $3, NOW() - interval '1 hour')`, f.campaign, f.leads[1], f.step); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.HoldLead(ctx, f.campaign, f.leads[2], nil, "away", "manual"); err != nil {
		t.Fatal(err)
	}

	got, err := repo.LeadSupply(ctx, f.campaign, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.DueNow != 2 || got.DueNowNewLeads != 2 || got.Held != 1 || got.DueLaterToday != 0 || got.WaitingOnStep != 0 {
		t.Fatalf("got %+v, want 2 due now (both new), 1 held", got)
	}

	// An entry delay moves every new lead past "now"; whether that is later
	// today or after today depends on where the day ends.
	if _, err := pool.Exec(ctx, `UPDATE campaigns SET entry_delay_minutes = 60 WHERE id = $1`, f.campaign); err != nil {
		t.Fatal(err)
	}
	got, err = repo.LeadSupply(ctx, f.campaign, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got.DueNow != 0 || got.DueLaterToday != 2 || got.DueLaterTodayNewLeads != 2 || got.NextDueAt == nil {
		t.Fatalf("with a 60-minute entry delay and a day ending in 2 hours, got %+v, want 2 due later today", got)
	}
	got, err = repo.LeadSupply(ctx, f.campaign, time.Now().Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got.DueNow != 0 || got.DueLaterToday != 0 || got.WaitingOnStep != 2 {
		t.Fatalf("with a 60-minute entry delay and a day ending in 30 minutes, got %+v, want 2 waiting on a step", got)
	}
}

func TestLiveCountCampaignSendsTodayBySenderMatchesTheMailboxLedger(t *testing.T) {
	_, pool := liveContactDB(t)
	f := newRoutedPairsFixture(t, pool, 1)
	ctx := context.Background()
	tasks := NewTaskRepository(pool)

	sent, wakeup, warmup := uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%q: %v", sql[:min(60, len(sql))], err)
		}
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM campaign_tasks WHERE task_id = ANY($1)`, []uuid.UUID{sent, wakeup, warmup})
		_, _ = pool.Exec(c, `DELETE FROM tasks WHERE id = ANY($1)`, []uuid.UUID{sent, wakeup, warmup})
	})
	// A real send, a bare wake-up of the same chain, and a warmup send.
	exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at) VALUES ($1, 'campaign', $2, 'completed', 'm-1', NOW())`, sent, f.mailbox)
	exec(`INSERT INTO campaign_tasks (task_id, campaign_id) VALUES ($1, $2)`, sent, f.campaign)
	exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at) VALUES ($1, 'campaign', $2, 'completed', '', NOW())`, wakeup, f.mailbox)
	exec(`INSERT INTO campaign_tasks (task_id, campaign_id) VALUES ($1, $2)`, wakeup, f.campaign)
	exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at) VALUES ($1, 'warmup', $2, 'completed', 'm-2', NOW())`, warmup, f.mailbox)

	bySender, err := tasks.CountCampaignSendsTodayBySender(ctx, f.campaign)
	if err != nil {
		t.Fatal(err)
	}
	all, err := tasks.CountCampaignEmailsSentToday(ctx, f.mailbox)
	if err != nil {
		t.Fatal(err)
	}
	if bySender[f.mailbox] != 1 || all != 1 {
		t.Fatalf("campaign ledger %v, mailbox ledger %d: want exactly the one real send in both", bySender, all)
	}
}
