package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

// The advisor's pool finding explains itself with the band's own reason (#491),
// and that reason only reaches it through this thirty-column select. Nothing
// else proves the columns and the scan stay in step.
func TestLiveAdvisorSnapshotCarriesThePoolStanding(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	requireSchemaVersion(t, pool, 157)

	user, org, account := uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(50, len(sql))], err)
		}
	}
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM warmup_pool_participants WHERE email_account_id = $1`,
			`DELETE FROM email_accounts WHERE id = $1`,
		} {
			if _, err := pool.Exec(ctx, sql, account); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, org); err != nil {
			t.Errorf("cleanup org: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	exec(`INSERT INTO users (id, email, first_name, last_name) VALUES ($1, $2, 'Advisor', 'Live')`,
		user, "adv-"+user.String()[:8]+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1, 'Advisor Live', $2, $3)`,
		org, "adv-"+org.String()[:8], user)
	exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html,
	          provider, status, campaign_limit, min_wait_time, timezone)
	      VALUES ($1, $2, $3, $4, 'Advisor', '', '', 'smtp_imap', 'active', 50, 600, 'UTC')`,
		account, user, org, "adv-"+account.String()[:8]+"@test.local")
	exec(`INSERT INTO warmup_pool_participants (pool_id, email_account_id, health_state, last_health_score, last_health_reason, blocked_at, blocked_until)
	      VALUES ($1, $2, 'quarantined', 41.5, 'warmup spam placement 24.0% exceeded quarantine threshold', NOW(), NOW() + INTERVAL '7 days')`,
		models.WarmupPoolPremiumID, account)

	snap, err := NewAdvisorRepository(handle).LoadSnapshot(ctx, org, time.Now())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var got *AdvisorMailbox
	for i := range snap.Mailboxes {
		if snap.Mailboxes[i].ID == account {
			got = &snap.Mailboxes[i]
		}
	}
	if got == nil {
		t.Fatal("the snapshot did not carry the mailbox at all")
	}
	if got.PoolHealth != "quarantined" || !got.PoolBlocked {
		t.Fatalf("pool standing = %q blocked=%v, want quarantined and blocked", got.PoolHealth, got.PoolBlocked)
	}
	if got.PoolHealthScore != 41.5 {
		t.Fatalf("pool health score = %v, want 41.5; the select and the scan have drifted", got.PoolHealthScore)
	}
	if got.PoolHealthReason != "warmup spam placement 24.0% exceeded quarantine threshold" {
		t.Fatalf("pool health reason = %q, want the band's own reason", got.PoolHealthReason)
	}
}

// The advisor counts a campaign's senders on ResolveCampaignSenderPool's terms:
// no pool and no tags means every active mailbox, unless it named its own.
func TestLiveAdvisorSnapshotCountsTheAllMailboxesFallback(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	requireSchemaVersion(t, pool, 157)

	user, org := uuid.New(), uuid.New()
	accounts := []uuid.UUID{uuid.New(), uuid.New()}
	fallback, explicit, picked := uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(50, len(sql))], err)
		}
	}
	t.Cleanup(func() {
		for _, sql := range []string{
			`DELETE FROM campaigns WHERE organization_id = $1`,
			`DELETE FROM email_accounts WHERE organization_id = $1`,
			`DELETE FROM organizations WHERE id = $1`,
		} {
			if _, err := pool.Exec(ctx, sql, org); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user); err != nil {
			t.Errorf("cleanup user: %v", err)
		}
	})

	exec(`INSERT INTO users (id, email, first_name, last_name) VALUES ($1, $2, 'Advisor', 'Live')`,
		user, "adv-"+user.String()[:8]+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1, 'Advisor Live', $2, $3)`,
		org, "adv-"+org.String()[:8], user)
	for _, a := range accounts {
		exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html,
		          provider, status, campaign_limit, min_wait_time, timezone)
		      VALUES ($1, $2, $3, $4, 'Advisor', '', '', 'smtp_imap', 'active', 30, 600, 'UTC')`,
			a, user, org, "adv-"+a.String()[:8]+"@test.local")
	}
	for id, strategy := range map[uuid.UUID]string{fallback: "tags", explicit: "explicit", picked: "tags"} {
		exec(`INSERT INTO campaigns (id, user_id, organization_id, name, description, days, status, sender_strategy, updated_at, created_at)
		      VALUES ($1, $2, $3, 'Advisor', '', 62, 'active', $4, NOW(), NOW())`, id, user, org, strategy)
	}
	exec(`INSERT INTO campaign_senders (campaign_id, email_account_id) VALUES ($1, $2)`, picked, accounts[0])

	snap, err := NewAdvisorRepository(handle).LoadSnapshot(ctx, org, time.Now())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	want := map[uuid.UUID][2]int{fallback: {2, 60}, explicit: {0, 0}, picked: {1, 30}}
	for _, c := range snap.Campaigns {
		w, ok := want[c.ID]
		if !ok {
			continue
		}
		if c.SenderCount != w[0] || c.SenderCapacity != w[1] {
			t.Errorf("campaign %s (%s): senders=%d capacity=%d, want %d and %d",
				c.ID, c.SenderStrategy, c.SenderCount, c.SenderCapacity, w[0], w[1])
		}
		if picked := map[bool]int{true: 1}[c.ID == picked]; c.PickedSenders != picked || c.SenderTags != 0 {
			t.Errorf("campaign %s: picked=%d tags=%d, want %d and 0", c.ID, c.PickedSenders, c.SenderTags, picked)
		}
		delete(want, c.ID)
	}
	if len(want) > 0 {
		t.Fatalf("the snapshot is missing %d campaigns", len(want))
	}
}
