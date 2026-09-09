package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/repository"
)

// Live checks for the campaign entry delay: a contact's FIRST email waits
// entry_delay_minutes after they entered the campaign. Skipped unless
// WARMBLY_TEST_DB is set; run against a migrated database with:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/scheduler/ -run TestLiveEntryDelay -v
//
// These need the real routing SQL: the delay is applied inside the finder's
// per-lead due check, which every unit test in this package stubs out.

// entryDelayFixture is an org/mailbox/campaign/contact graph with one lead whose
// entry time and the campaign's delay are both under the test's control.
type entryDelayFixture struct {
	pool     *pgxpool.Pool
	user     uuid.UUID
	org      uuid.UUID
	mailbox  uuid.UUID
	campaign uuid.UUID
	contact  uuid.UUID
	step     uuid.UUID
}

func newEntryDelayFixture(t *testing.T, pool *pgxpool.Pool, delayMinutes int) *entryDelayFixture {
	t.Helper()
	ctx := context.Background()
	f := &entryDelayFixture{
		pool: pool, user: uuid.New(), org: uuid.New(), mailbox: uuid.New(),
		campaign: uuid.New(), contact: uuid.New(), step: uuid.New(),
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}

	exec(`INSERT INTO users (id, email, first_name, last_name)
	      VALUES ($1, $2, 'Entry', 'Delay')`, f.user, "entry-"+f.user.String()[:8]+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Entry Delay', $2, $3)`, f.org, "entry-"+f.org.String()[:8], f.user)
	exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name,
	          signature_plain, signature_html, provider, status, campaign_limit, min_wait_time, timezone)
	      VALUES ($1, $2, $3, $4, 'Entry', '', '', 'smtp_imap', 'active', 50, 0, 'UTC')`,
		f.mailbox, f.user, f.org, "entry-"+f.mailbox.String()[:8]+"@test.local")
	// An always-open window, so the only thing that can hold the send back is
	// the entry delay under test.
	exec(`INSERT INTO campaigns (id, user_id, organization_id, name, description, status,
	          daily_limit, timezone, days, start_time, end_time, rotation_mode,
	          entry_delay_minutes, updated_at, created_at)
	      VALUES ($1, $2, $3, 'Entry Delay', '', 'active', 50, 'UTC', 127, '00:00', '23:59',
	              'least_recently_used', $4, NOW(), NOW())`, f.campaign, f.user, f.org, delayMinutes)
	exec(`INSERT INTO sequences (id, campaign_id, organization_id, name, subject,
	          body_plain, body_html, wait_after, position, kind)
	      VALUES ($1, $2, $3, 'Step 1', 'Hi', 'Hello', '<p>Hello</p>', 0, 0, 'email')`,
		f.step, f.campaign, f.org)
	exec(`INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields)
	      VALUES ($1, $2, $3, $4, 'Entry', 'Contact', '', '', '{}')`,
		f.contact, f.user, f.org, "lead-"+f.contact.String()[:8]+"@test.local")
	exec(`INSERT INTO campaign_leads (campaign_id, contact_id, position) VALUES ($1, $2, 0)`, f.campaign, f.contact)

	t.Cleanup(func() {
		c := context.Background()
		steps := []struct {
			sql string
			arg any
		}{
			{`DELETE FROM campaign_tasks WHERE task_id IN (SELECT id FROM tasks WHERE email_account_id = $1)`, f.mailbox},
			{`DELETE FROM tasks WHERE email_account_id = $1`, f.mailbox},
			{`DELETE FROM campaign_contact_progress WHERE campaign_id = $1`, f.campaign},
			{`DELETE FROM campaign_leads WHERE campaign_id = $1`, f.campaign},
			{`DELETE FROM sequences WHERE campaign_id = $1`, f.campaign},
			{`DELETE FROM campaign_logs WHERE campaign_id = $1`, f.campaign},
			{`DELETE FROM campaign_daily_sends WHERE campaign_id = $1`, f.campaign},
			{`DELETE FROM campaigns WHERE id = $1`, f.campaign},
			{`DELETE FROM email_account_daily_plan WHERE email_account_id = $1`, f.mailbox},
			{`DELETE FROM email_account_behavior WHERE email_account_id = $1`, f.mailbox},
			{`DELETE FROM email_accounts WHERE id = $1`, f.mailbox},
			{`DELETE FROM contacts WHERE organization_id = $1`, f.org},
			{`DELETE FROM organizations WHERE id = $1`, f.org},
			{`DELETE FROM users WHERE id = $1`, f.user},
		}
		for _, step := range steps {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})
	return f
}

// enteredAt moves when the lead entered the campaign. A nil value writes NULL,
// which is what a lead added before the column existed looks like.
func (f *entryDelayFixture) enteredAt(t *testing.T, at *time.Time) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE campaign_leads SET added_at = $3 WHERE campaign_id = $1 AND contact_id = $2`,
		f.campaign, f.contact, at); err != nil {
		t.Fatalf("set added_at: %v", err)
	}
}

func (f *entryDelayFixture) campaignCreatedAt(t *testing.T, at time.Time) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE campaigns SET created_at = $2 WHERE id = $1`, f.campaign, at); err != nil {
		t.Fatalf("set created_at: %v", err)
	}
}

func (f *entryDelayFixture) route(t *testing.T) *repository.ContactRoute {
	t.Helper()
	route, err := repository.NewCampaignProgressRepository(f.pool).
		RouteContact(context.Background(), f.campaign, f.contact)
	if err != nil {
		t.Fatalf("route contact: %v", err)
	}
	return route
}

// TestLiveEntryDelayHoldsTheFirstEmail is the headline check: with a two-day
// delay and a lead that entered a moment ago, the scheduler must refuse to hand
// back a sendable pair and must report the moment the email becomes due.
func TestLiveEntryDelayHoldsTheFirstEmail(t *testing.T) {
	handle, pool := liveDB(t)
	f := newEntryDelayFixture(t, pool, 2*24*60)
	entered := time.Now().UTC().Add(-time.Minute)
	f.enteredAt(t, &entered)

	s := liveScheduler(t, handle, pool)
	at, pair, _, err := s.CalculateNextCampaignTime(context.Background(), f.campaign)
	if !errors.Is(err, ErrCampaignDeferred) {
		t.Fatalf("want ErrCampaignDeferred while the entry delay is running, got pair=%v err=%v", pair, err)
	}
	if pair != nil {
		t.Fatalf("a deferred pass must never hand back a sendable pair, got %+v", pair)
	}
	want := entered.Add(48 * time.Hour)
	if at.Sub(want).Abs() > time.Minute {
		t.Fatalf("re-check at %s, want the delay's end %s", at.UTC(), want)
	}

	route := f.route(t)
	if route.Target == nil || *route.Target != f.step {
		t.Fatalf("routing should still point at the first step, got %v", route.Target)
	}
	if route.DueAt == nil || route.DueAt.Sub(want).Abs() > time.Minute {
		t.Fatalf("due at %v, want %s", route.DueAt, want)
	}

	pv, perr := s.(ContactSendPreviewer).PreviewContactSend(context.Background(), f.campaign, f.contact)
	if perr != nil {
		t.Fatalf("preview: %v", perr)
	}
	if pv.Constraint != ConstraintEntryDelay {
		t.Fatalf("preview constraint %q, want %q", pv.Constraint, ConstraintEntryDelay)
	}
	// The preview reports the earliest REAL slot, so it is the delay's end
	// pushed forward by the mailbox's own hours and the day's pacing — never
	// anything before it.
	if pv.NotBefore == nil || pv.NotBefore.Before(want.Add(-time.Minute)) {
		t.Fatalf("preview not-before %v, want the delay's end (%s) or later", pv.NotBefore, want)
	}
	t.Logf("held until %s, %s after the contact entered", at.UTC().Format(time.RFC3339), 48*time.Hour)
}

// TestLiveEntryDelayReleasesAfterItElapses proves the same campaign sends once
// the delay has passed for that contact — the delay is per lead, not per campaign.
func TestLiveEntryDelayReleasesAfterItElapses(t *testing.T) {
	handle, pool := liveDB(t)
	f := newEntryDelayFixture(t, pool, 2*24*60)
	entered := time.Now().UTC().Add(-72 * time.Hour)
	f.enteredAt(t, &entered)

	_, pair, accountID, err := liveScheduler(t, handle, pool).
		CalculateNextCampaignTime(context.Background(), f.campaign)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if pair == nil || pair.ContactID != f.contact || pair.SequenceID != f.step {
		t.Fatalf("want the fixture's first step for the lead, got %+v", pair)
	}
	if !pair.IsNewLead {
		t.Fatal("the first step of a lead that has never been sent is a new lead")
	}
	if accountID != f.mailbox {
		t.Fatalf("picked mailbox %s, want %s", accountID, f.mailbox)
	}
}

// TestLiveEntryDelayIgnoredWithoutOne is the regression guard for every existing
// campaign: with no delay set, a lead that entered a moment ago is due now.
func TestLiveEntryDelayIgnoredWithoutOne(t *testing.T) {
	handle, pool := liveDB(t)
	f := newEntryDelayFixture(t, pool, 0)

	// Deliberately does NOT write added_at: the fixture inserts the lead the
	// way every production path does (column omitted), so this also proves the
	// column's now() default fires instead of leaving a NULL behind.
	var addedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT added_at FROM campaign_leads WHERE campaign_id = $1 AND contact_id = $2`,
		f.campaign, f.contact).Scan(&addedAt); err != nil {
		t.Fatalf("read added_at: %v", err)
	}
	if addedAt == nil || time.Since(*addedAt) > time.Minute {
		t.Fatalf("added_at = %v, want the insert's own timestamp", addedAt)
	}

	_, pair, _, err := liveScheduler(t, handle, pool).
		CalculateNextCampaignTime(context.Background(), f.campaign)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if pair == nil {
		t.Fatal("a campaign with no entry delay must send a brand-new lead immediately")
	}
	if route := f.route(t); route.DueAt != nil {
		t.Fatalf("no delay means no due-at, got %s", route.DueAt)
	}
}

// TestLiveEntryDelayFallsBackToCampaignCreation covers the leads that predate
// the added_at column (NULL): they count from the campaign's own creation time,
// so turning a delay on never re-delays a lead enrolled weeks ago.
func TestLiveEntryDelayFallsBackToCampaignCreation(t *testing.T) {
	handle, pool := liveDB(t)
	f := newEntryDelayFixture(t, pool, 2*24*60)
	f.enteredAt(t, nil)
	f.campaignCreatedAt(t, time.Now().UTC().Add(-30*24*time.Hour))

	_, pair, _, err := liveScheduler(t, handle, pool).
		CalculateNextCampaignTime(context.Background(), f.campaign)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if pair == nil {
		t.Fatal("a lead with no recorded entry time and a month-old campaign is past any 2-day delay")
	}

	// The same NULL on a campaign created just now is still inside the delay.
	f.campaignCreatedAt(t, time.Now().UTC())
	_, pair, _, err = liveScheduler(t, handle, pool).
		CalculateNextCampaignTime(context.Background(), f.campaign)
	if !errors.Is(err, ErrCampaignDeferred) || pair != nil {
		t.Fatalf("want the delay to hold from a fresh campaign's creation time, got pair=%v err=%v", pair, err)
	}
}

// TestLiveEntryDelayOutlivesTodaysNewLeadCap is the completion guard. With the
// daily new-lead cap already spent, the finder skips new leads BEFORE routing
// them, so a delayed lead contributes no re-check time to that pass. Without the
// unexcluded pass handing its own re-check back, a campaign whose only remaining
// leads are inside the entry delay reports itself COMPLETE and stops.
func TestLiveEntryDelayOutlivesTodaysNewLeadCap(t *testing.T) {
	handle, pool := liveDB(t)
	ctx := context.Background()
	f := newEntryDelayFixture(t, pool, 2*24*60)
	entered := time.Now().UTC()
	f.enteredAt(t, &entered)

	// A cap of one, already spent for today, so the delayed lead is the only
	// one left and the finder's excluded pass skips it before routing it.
	if _, err := pool.Exec(ctx, `UPDATE campaigns SET max_new_leads_per_day = 1 WHERE id = $1`, f.campaign); err != nil {
		t.Fatalf("set cap: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_daily_sends (campaign_id, send_date, emails_sent, new_leads_started)
	      VALUES ($1, CURRENT_DATE, 1, 1)`, f.campaign); err != nil {
		t.Fatalf("spend the cap: %v", err)
	}

	at, pair, _, err := liveScheduler(t, handle, pool).CalculateNextCampaignTime(ctx, f.campaign)
	if !errors.Is(err, ErrCampaignDeferred) {
		t.Fatalf("want a deferral while a delayed lead is still waiting, got pair=%v err=%v", pair, err)
	}
	if want := entered.Add(48 * time.Hour); at.Sub(want).Abs() > time.Minute {
		t.Fatalf("re-check at %s, want the delayed lead's due time %s", at.UTC(), want)
	}
}

// TestLiveEntryDelayDoesNotBlockDueFollowUps proves the delay applies to the
// FIRST step only: a lead already inside the sequence keeps moving on its own
// step waits while a brand-new lead is still held.
func TestLiveEntryDelayDoesNotBlockDueFollowUps(t *testing.T) {
	handle, pool := liveDB(t)
	ctx := context.Background()
	f := newEntryDelayFixture(t, pool, 7*24*60)
	fresh := time.Now().UTC()
	f.enteredAt(t, &fresh)

	// A second step, connected from the first with no wait, and a second lead
	// who already received step one.
	followUp := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sequences (id, campaign_id, organization_id, name, subject,
	        body_plain, body_html, wait_after, position, kind)
	      VALUES ($1, $2, $3, 'Step 2', 'Again', 'Hello again', '<p>Hello again</p>', 0, 1, 'email')`,
		followUp, f.campaign, f.org); err != nil {
		t.Fatalf("follow-up step: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sequences SET conditions = $2 WHERE id = $1`, f.step,
		`{"branches":[{"branch_id":"b1","target_step_id":"`+followUp.String()+`"}]}`); err != nil {
		t.Fatalf("connect steps: %v", err)
	}

	veteran := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO contacts (id, user_id, organization_id, email, first_name, last_name, company, phone, custom_fields)
	      VALUES ($1, $2, $3, $4, 'Veteran', 'Lead', '', '', '{}')`,
		veteran, f.user, f.org, "vet-"+veteran.String()[:8]+"@test.local"); err != nil {
		t.Fatalf("veteran contact: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_leads (campaign_id, contact_id, position, added_at)
	      VALUES ($1, $2, 1, NOW() - INTERVAL '10 days')`, f.campaign, veteran); err != nil {
		t.Fatalf("veteran lead: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, sent_at, dispatched_at)
	      VALUES ($1, $2, $3, NOW() - INTERVAL '5 days', NOW() - INTERVAL '5 days')`,
		f.campaign, veteran, f.step); err != nil {
		t.Fatalf("veteran progress: %v", err)
	}

	_, pair, _, err := liveScheduler(t, handle, pool).CalculateNextCampaignTime(ctx, f.campaign)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if pair == nil {
		t.Fatal("the due follow-up must still be sendable while a new lead waits out the entry delay")
	}
	if pair.ContactID != veteran || pair.SequenceID != followUp {
		t.Fatalf("want the veteran's follow-up, got %+v", pair)
	}
}
