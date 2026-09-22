package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Issue #558: completed campaign tasks must drive the mailbox's sent-today count.
func TestLiveAccountDailyUsageCountsCampaignTasks(t *testing.T) {
	handle, pool := liveContactDB(t)
	ctx := context.Background()
	userID, orgID, accountID := uuid.New(), uuid.New(), uuid.New()
	day := time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}

	exec(`INSERT INTO users (id, first_name, last_name, email)
	      VALUES ($1, 'Issue', '558', $2)`, userID, "issue-558-"+uuid.NewString()+"@example.test")
	exec(`INSERT INTO organizations (id, name, owner_user_id)
	      VALUES ($1, 'Issue 558', $2)`, orgID, userID)
	exec(`INSERT INTO email_accounts
	        (id, user_id, organization_id, email, name, signature_plain, signature_html, provider, campaign_limit)
	      VALUES ($1, $2, $3, $4, 'Issue 558', '', '', 'smtp_imap', 75)`,
		accountID, userID, orgID, "mailbox-"+uuid.NewString()+"@example.test")
	exec(`INSERT INTO tasks
	        (id, task_type, email_account_id, status, message_id, completed_at)
	      VALUES ($1, 'campaign', $2, 'completed', '<sent@example.test>', $3)`, uuid.New(), accountID, day)

	var taskSends, legacyCounts int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tasks t
		WHERE t.email_account_id = $1
		  AND t.status = 'completed'
		  AND t.task_type = 'campaign'
		  AND DATE(t.completed_at) = $2::date
		  AND `+taskDispatchedEmail, accountID, day).Scan(&taskSends); err != nil {
		t.Fatalf("count task sends: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM daily_email_counts
		WHERE email_account_id = $1 AND date = $2::date`, accountID, day).Scan(&legacyCounts); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	if taskSends != 1 || legacyCounts != 0 {
		t.Fatalf("fixture ledgers = %d task sends, %d legacy rows; want 1/0", taskSends, legacyCounts)
	}

	t.Cleanup(func() {
		for _, step := range []struct {
			query string
			arg   uuid.UUID
		}{
			{`DELETE FROM email_accounts WHERE id = $1`, accountID},
			{`DELETE FROM organizations WHERE id = $1`, orgID},
			{`DELETE FROM users WHERE id = $1`, userID},
		} {
			if _, err := pool.Exec(context.Background(), step.query, step.arg); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})

	repo := &analyticsRepository{DB: handle}
	usage, xerr := repo.GetAccountDailyUsage(ctx, accountID, day)
	if xerr != nil {
		t.Fatalf("GetAccountDailyUsage: %v", xerr)
	}
	if usage.CampaignSent != 1 {
		t.Fatalf("campaign_sent = %d, want 1 completed campaign send", usage.CampaignSent)
	}
	if usage.CampaignLimit != 75 {
		t.Fatalf("campaign_limit = %d, want 75", usage.CampaignLimit)
	}
}
