package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A stored Sent row must never stamp campaign progress as replied (issue #549).
func TestLiveReplySourceUsesStoredDirectionAtTheWriteBoundary(t *testing.T) {
	_, pool := liveContactDB(t)
	f := newThreadParentFixture(t, pool)
	step := f.step(1, "Hello", true)
	parentTask := f.send(step, f.mailbox, "<opener@test.local>", "thread-1", 60)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, dispatch_task_id, sent_at)
		VALUES ($1, $2, $3, $4, NOW() - INTERVAL '1 minute')
	`, f.campaign, f.contact, step, parentTask); err != nil {
		t.Fatalf("insert progress: %v", err)
	}

	sentID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO unibox_emails (id, user_id, email_id, folder, provider_folder)
		VALUES ($1, $2, $3, 'sent', 'sent')
	`, sentID, f.owner, f.other); err != nil {
		t.Fatalf("insert sent source: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM unibox_emails WHERE id = $1`, sentID); err != nil {
			t.Errorf("cleanup sent source: %v", err)
		}
	})

	repo := NewCampaignProgressRepository(pool)
	inbound, err := repo.IsInboundReplySource(ctx, f.other, sentID)
	if err != nil {
		t.Fatalf("verify sent source: %v", err)
	}
	if inbound {
		t.Fatal("Sent-folder source was accepted as inbound")
	}
	claimToken, err := repo.ClaimIncomingReply(ctx, f.other, sentID)
	if err != nil {
		t.Fatalf("claim sent source: %v", err)
	}
	if claimToken != uuid.Nil {
		t.Fatal("Sent-folder source was claimed for reply processing")
	}
	accepted, err := repo.RecordEmailReplied(ctx, f.campaign, f.contact, step, f.other, sentID)
	if err != nil {
		t.Fatalf("record sent source: %v", err)
	}
	if accepted {
		t.Fatal("Sent-folder source was accepted as reply progress")
	}

	var replied bool
	if err := pool.QueryRow(ctx, `
		SELECT replied_at IS NOT NULL
		FROM campaign_contact_progress
		WHERE campaign_id = $1 AND contact_id = $2 AND sequence_id = $3
	`, f.campaign, f.contact, step).Scan(&replied); err != nil {
		t.Fatalf("read progress: %v", err)
	}
	if replied {
		t.Fatal("Sent-folder source stamped replied_at")
	}

	inboxID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO unibox_emails (id, user_id, email_id, folder, provider_folder)
		VALUES ($1, $2, $3, 'inbox', 'inbox')
	`, inboxID, f.owner, f.mailbox); err != nil {
		t.Fatalf("insert inbound source: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM unibox_emails WHERE id = $1`, inboxID); err != nil {
			t.Errorf("cleanup inbound source: %v", err)
		}
	})

	inbound, err = repo.IsInboundReplySource(ctx, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("verify inbound source: %v", err)
	}
	if !inbound {
		t.Fatal("Inbox source was rejected as outbound")
	}
	claimToken, err = repo.ClaimIncomingReply(ctx, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("claim inbound source: %v", err)
	}
	if claimToken == uuid.Nil {
		t.Fatal("Inbox source was not claimed for reply processing")
	}
	firstClaimToken := claimToken
	claimToken, err = repo.ClaimIncomingReply(ctx, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("repeat inbound claim: %v", err)
	}
	if claimToken != uuid.Nil {
		t.Fatal("Inbox source was claimed concurrently twice")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE unibox_emails
		SET campaign_reply_claimed_at = NOW() - INTERVAL '11 minutes'
		WHERE id = $1
	`, inboxID); err != nil {
		t.Fatalf("age reply claim: %v", err)
	}
	claimToken, err = repo.ClaimIncomingReply(ctx, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
	if claimToken == uuid.Nil {
		t.Fatal("Expired reply-processing lease was not reclaimed")
	}
	if err := repo.CompleteIncomingReply(ctx, f.mailbox, inboxID, firstClaimToken); err == nil {
		t.Fatal("Expired claim completed work owned by its replacement")
	}
	accepted, err = repo.RecordEmailReplied(ctx, f.campaign, f.contact, step, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("record inbound source: %v", err)
	}
	if !accepted {
		t.Fatal("Inbox source did not stamp reply progress")
	}
	accepted, err = repo.RecordEmailReplied(ctx, f.campaign, f.contact, step, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("repeat inbound record: %v", err)
	}
	if !accepted {
		t.Fatal("Already-recorded reply was not accepted idempotently")
	}
	if err := repo.CompleteIncomingReply(ctx, f.mailbox, inboxID, claimToken); err != nil {
		t.Fatalf("complete inbound reply: %v", err)
	}
	if err := repo.CompleteIncomingReply(ctx, f.mailbox, inboxID, claimToken); err == nil {
		t.Fatal("Completed claim was accepted twice")
	}
	claimToken, err = repo.ClaimIncomingReply(ctx, f.mailbox, inboxID)
	if err != nil {
		t.Fatalf("claim completed reply: %v", err)
	}
	if claimToken != uuid.Nil {
		t.Fatal("Completed reply was claimed again")
	}
	if err := pool.QueryRow(ctx, `
		SELECT replied_at IS NOT NULL
		FROM campaign_contact_progress
		WHERE campaign_id = $1 AND contact_id = $2 AND sequence_id = $3
	`, f.campaign, f.contact, step).Scan(&replied); err != nil {
		t.Fatalf("read inbound progress: %v", err)
	}
	if !replied {
		t.Fatal("Inbox source did not stamp replied_at")
	}
}

// A cross-provider reply is eligible only when its receiving mailbox sent to the lead.
func TestLiveCrossProviderReplyRequiresReceivingMailboxUsedForLead(t *testing.T) {
	_, pool := liveContactDB(t)
	f := newThreadParentFixture(t, pool)
	ctx := context.Background()
	root := f.step(1, "Hello", true)
	rootTask := f.send(root, f.mailbox, "<opener@test.local>", "thread-1", 120)

	if _, err := pool.Exec(ctx, `
		UPDATE email_accounts
		SET provider = CASE id WHEN $1 THEN 'outlook'::email_provider ELSE 'smtp_imap'::email_provider END
		WHERE id IN ($1, $2)
	`, f.mailbox, f.other); err != nil {
		t.Fatalf("set cross-provider fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, dispatch_task_id, sent_at)
		VALUES ($1, $2, $3, $4, NOW() - INTERVAL '2 minutes')
	`, f.campaign, f.contact, root, rootTask); err != nil {
		t.Fatalf("insert root progress: %v", err)
	}

	repo := NewCampaignProgressRepository(pool)
	used, err := repo.CampaignContactSentFromAccount(ctx, f.campaign, f.contact, f.other)
	if err != nil {
		t.Fatalf("check unused SMTP/IMAP mailbox: %v", err)
	}
	if used {
		t.Fatal("unused workspace mailbox was accepted for a cross-mailbox reply")
	}

	followup := f.step(2, "Re: Hello", true)
	followupTask := f.send(followup, f.other, "<followup@test.local>", "thread-1", 60)
	if _, err := pool.Exec(ctx, `
		INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, dispatch_task_id, sent_at)
		VALUES ($1, $2, $3, $4, NOW() - INTERVAL '1 minute')
	`, f.campaign, f.contact, followup, followupTask); err != nil {
		t.Fatalf("insert cross-provider progress: %v", err)
	}

	used, err = repo.CampaignContactSentFromAccount(ctx, f.campaign, f.contact, f.other)
	if err != nil {
		t.Fatalf("check used SMTP/IMAP mailbox: %v", err)
	}
	if !used {
		t.Fatal("mailbox that sent a later campaign step was rejected for a cross-provider reply")
	}
}
