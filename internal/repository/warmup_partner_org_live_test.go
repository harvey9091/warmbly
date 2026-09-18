package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// partnerOrgFixture supplies sibling and outside partner ownership.
type partnerOrgFixture struct {
	pool   *pgxpool.Pool
	user   uuid.UUID
	org    uuid.UUID
	other  uuid.UUID
	sender uuid.UUID
	// sibling and outside distinguish the two ownership tiers.
	sibling uuid.UUID
	outside uuid.UUID
	exec    func(sql string, args ...any)
}

func newPartnerOrgFixture(t *testing.T) *partnerOrgFixture {
	t.Helper()
	_, pool := liveContactDB(t)
	ctx := context.Background()
	f := &partnerOrgFixture{
		pool: pool, user: uuid.New(), org: uuid.New(), other: uuid.New(),
		sender: uuid.New(), sibling: uuid.New(), outside: uuid.New(),
	}
	f.exec = func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	f.exec(`INSERT INTO users (id, email, first_name, last_name) VALUES ($1, $2, 'Org', 'Pairing')`,
		f.user, "org-pair-"+f.user.String()[:8]+"@test.local")
	for _, org := range []uuid.UUID{f.org, f.other} {
		f.exec(`INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1, 'Org pairing', $2, $3)`,
			org, "org-pair-"+org.String()[:8], f.user)
	}
	for _, m := range []struct {
		id     uuid.UUID
		org    uuid.UUID
		domain string
	}{
		{f.sender, f.org, "own-a.test"},
		{f.sibling, f.org, "own-b.test"},
		{f.outside, f.other, "elsewhere.test"},
	} {
		f.exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain,
		            signature_html, provider, status, campaign_limit, min_wait_time, timezone)
		        VALUES ($1, $2, $3, $4, 'Org pairing', '', '', 'smtp_imap', 'active', 50, 600, 'UTC')`,
			m.id, f.user, m.org, "box-"+m.id.String()[:8]+"@"+m.domain)
	}
	for _, id := range []uuid.UUID{f.sender, f.sibling, f.outside} {
		f.exec(`INSERT INTO warmup_pool_participants (pool_id, email_account_id, participant_role, health_state)
		        VALUES ($1, $2, 'sender_receiver', 'healthy')`, premiumPoolID, id)
	}
	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM warmup_tokens WHERE sender_account_id = $1`, f.sender},
			{`DELETE FROM tasks WHERE email_account_id = $1`, f.sender},
			{`DELETE FROM warmup_pool_participants WHERE email_account_id IN
			    (SELECT id FROM email_accounts WHERE user_id = $1)`, f.user},
			{`DELETE FROM email_accounts WHERE user_id = $1`, f.user},
			{`DELETE FROM organizations WHERE owner_user_id = $1`, f.user},
			{`DELETE FROM users WHERE id = $1`, f.user},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

// warmed records one warmup send from the sender to a recipient.
func (f *partnerOrgFixture) warmed(t *testing.T, recipient uuid.UUID) {
	t.Helper()
	f.attempted(t, recipient, "completed")
}

// attempted records a token and its task outcome.
func (f *partnerOrgFixture) attempted(t *testing.T, recipient uuid.UUID, status string) {
	t.Helper()
	taskID := uuid.New()
	f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id)
	        VALUES ($1, 'warmup', $2, $3, '')`, taskID, f.sender, status)
	f.exec(`INSERT INTO warmup_tokens (token, task_id, sender_account_id, recipient_account_id, sent_message_id, created_at)
	        VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW())`, taskID, f.sender, recipient, "<"+taskID.String()+"@test.local>")
}

// Candidates retain siblings while carrying ownership for later ranking.
func TestLiveWarmupPartnerCandidatesCarryTheirOwner(t *testing.T) {
	f := newPartnerOrgFixture(t)
	repo := &warmupRepository{db: f.pool}

	cands, err := repo.WarmupPartnerCandidates(context.Background(), "premium", f.sender)
	if err != nil {
		t.Fatalf("WarmupPartnerCandidates: %v", err)
	}
	seen := map[uuid.UUID]*uuid.UUID{}
	for _, c := range cands {
		seen[c.ID] = c.OrganizationID
	}
	if _, ok := seen[f.sender]; ok {
		t.Fatal("the sender is its own candidate")
	}
	for _, tc := range []struct {
		name string
		id   uuid.UUID
		want uuid.UUID
	}{
		{"sibling", f.sibling, f.org},
		{"outside partner", f.outside, f.other},
	} {
		got, ok := seen[tc.id]
		if !ok {
			t.Fatalf("%s is missing from the candidate set", tc.name)
		}
		if got == nil || *got != tc.want {
			t.Fatalf("%s owner = %v, want %s", tc.name, got, tc.want)
		}
	}
}

// Diversity counts distinct confirmed partner reach.
func TestLiveGetPartnerDiversityCountsDistinctPartners(t *testing.T) {
	f := newPartnerOrgFixture(t)
	ctx := context.Background()
	repo := &warmupRepository{db: f.pool}
	since := time.Now().Add(-7 * 24 * time.Hour)

	if d, err := repo.GetPartnerDiversity(ctx, f.sender, since); err != nil {
		t.Fatalf("GetPartnerDiversity on a mailbox that has sent nothing: %v", err)
	} else if d.Mailboxes != 0 || d.Domains != 0 || d.Organizations != 0 {
		t.Fatalf("a mailbox that sent nothing reads %+v, want zeros", d)
	}

	for _, status := range []string{"completed", "failed"} {
		taskID := uuid.New()
		f.exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id)
		        VALUES ($1, 'warmup', $2, $3, '')`, taskID, f.sender, status)
		f.exec(`INSERT INTO warmup_tokens (token, task_id, sender_account_id, recipient_account_id)
		        VALUES (gen_random_uuid(), $1, $2, $3)`, taskID, f.sender, f.sibling)
	}
	if d, err := repo.GetPartnerDiversity(ctx, f.sender, since); err != nil {
		t.Fatalf("GetPartnerDiversity on unconfirmed sends: %v", err)
	} else if d.Mailboxes != 0 || d.Domains != 0 || d.Organizations != 0 {
		t.Fatalf("unconfirmed sends read %+v, want zeros", d)
	}

	// Repeated sends to one sibling remain one distinct partner.
	for i := 0; i < 3; i++ {
		f.warmed(t, f.sibling)
	}
	d, err := repo.GetPartnerDiversity(ctx, f.sender, since)
	if err != nil {
		t.Fatalf("GetPartnerDiversity: %v", err)
	}
	if d.Mailboxes != 1 || d.Domains != 1 || d.Organizations != 1 {
		t.Fatalf("three sends to one sibling read %+v, want 1/1/1", d)
	}

	f.warmed(t, f.outside)
	d, err = repo.GetPartnerDiversity(ctx, f.sender, since)
	if err != nil {
		t.Fatalf("GetPartnerDiversity: %v", err)
	}
	if d.Mailboxes != 2 || d.Domains != 2 || d.Organizations != 2 {
		t.Fatalf("after an outside partner: %+v, want 2/2/2", d)
	}

	// A failed token cannot credit a partner that received no mail.
	third := uuid.New()
	f.exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain,
	            signature_html, provider, status, campaign_limit, min_wait_time, timezone)
	        VALUES ($1, $2, $3, $4, 'Third', '', '', 'smtp_imap', 'active', 50, 600, 'UTC')`,
		third, f.user, f.other, "third-"+third.String()[:8]+"@elsewhere-two.test")
	f.attempted(t, third, "failed")
	d, err = repo.GetPartnerDiversity(ctx, f.sender, since)
	if err != nil {
		t.Fatalf("GetPartnerDiversity: %v", err)
	}
	if d.Mailboxes != 2 || d.Domains != 2 || d.Organizations != 2 {
		t.Fatalf("a refused send counted as diversity: %+v, want 2/2/2", d)
	}

	// Outside the window nothing counts, so the number is about this week.
	if d, err := repo.GetPartnerDiversity(ctx, f.sender, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("GetPartnerDiversity: %v", err)
	} else if d.Mailboxes != 0 {
		t.Fatalf("a future window counted %d partners", d.Mailboxes)
	}
}
