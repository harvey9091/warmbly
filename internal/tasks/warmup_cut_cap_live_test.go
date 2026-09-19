package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	warmupapp "github.com/warmbly/warmbly/internal/app/warmup"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/encrypt"
	"github.com/warmbly/warmbly/internal/repository"
	"github.com/warmbly/warmbly/internal/scheduler"
	"github.com/warmbly/warmbly/internal/tasks/proto"
)

// The day's target is enforced at the moment a warmup send executes, not only
// when the next one is placed (#592). Run with WARMBLY_TEST_DB on a scratch
// database; the premium pool must be empty so the recipient cap is known.

type capFixture struct {
	pool     *pgxpool.Pool
	svc      *tasksService
	sender   *recordingSender
	user     uuid.UUID
	org      uuid.UUID
	mailbox  uuid.UUID
	partners []uuid.UUID
}

const capFixturePartners = 10

func newCapFixture(t *testing.T) *capFixture {
	t.Helper()
	handle := liveCampaignDB(t)
	pool := handle.Pool
	requireEmptyPool(t, pool, models.WarmupPoolPremiumID)
	requireEmptyPool(t, pool, models.WarmupPoolFreeID)

	f := &capFixture{pool: pool, sender: &recordingSender{}, user: uuid.New(), org: uuid.New(), mailbox: uuid.New()}
	t.Cleanup(func() {
		c := context.Background()
		for _, step := range []struct {
			sql string
			arg any
		}{
			{`DELETE FROM warmup_tokens WHERE sender_account_id = $1`, f.mailbox},
			{`DELETE FROM warmup_spam_reports WHERE reported_account_id = $1`, f.mailbox},
			{`DELETE FROM warmup_statistics WHERE email_account_id = $1`, f.mailbox},
			{`DELETE FROM warmup_tasks WHERE task_id IN (SELECT id FROM tasks WHERE email_account_id = $1)`, f.mailbox},
			{`DELETE FROM task_failures WHERE task_id IN (SELECT id FROM tasks WHERE email_account_id = $1)`, f.mailbox},
			{`DELETE FROM tasks WHERE email_account_id = $1`, f.mailbox},
			{`DELETE FROM warmup_pool_participants WHERE email_account_id IN (SELECT id FROM email_accounts WHERE organization_id = $1)`, f.org},
			{`DELETE FROM email_accounts WHERE organization_id = $1`, f.org},
			{`DELETE FROM organizations WHERE id = $1`, f.org},
			{`DELETE FROM users WHERE id = $1`, f.user},
		} {
			if _, err := pool.Exec(c, step.sql, step.arg); err != nil {
				t.Errorf("cleanup %q: %v", step.sql, err)
			}
		}
	})

	f.exec(t, `INSERT INTO users (id, email, first_name, last_name) VALUES ($1, $2, 'Cap', 'Test')`,
		f.user, "cap-"+f.user.String()[:8]+"@test.local")
	f.exec(t, `INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1, 'Cap Test', $2, $3)`,
		f.org, "cap-"+f.org.String()[:8], f.user)
	// Day one of a base-10 ramp, anchored with the database clock the way the
	// app anchors it. An always-open window keeps the clock out of the result.
	f.exec(t, `INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html,
	          provider, status, campaign_limit, min_wait_time, timezone, warmup, warmup_base, warmup_increase,
	          warmup_max, warmup_reply_rate, warmup_pool_type, warmup_start_time, warmup_end_time)
	      VALUES ($1, $2, $3, $4, 'Cap', '', '', 'smtp_imap', 'active', 50, 0, 'UTC', now(), 10, 1, 40, 0,
	              'premium', '00:00', '23:59')`,
		f.mailbox, f.user, f.org, "cap-"+f.mailbox.String()[:8]+"@test.local")
	// Enough recipients that the recipient cap sits above the ramp target.
	for i := 0; i < capFixturePartners; i++ {
		id := uuid.New()
		f.partners = append(f.partners, id)
		f.exec(t, `INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html,
		          provider, status, campaign_limit, min_wait_time, timezone, warmup_pool_type)
		      VALUES ($1, $2, $3, $4, 'Cap', '', '', 'smtp_imap', 'active', 50, 600, 'UTC', 'premium')`,
			id, f.user, f.org, "cap-"+id.String()[:8]+"@partner.test")
		f.exec(t, `INSERT INTO warmup_pool_participants (pool_id, email_account_id, participant_role, health_state)
		      VALUES ($1, $2, 'recipient_only', 'healthy')`, models.WarmupPoolPremiumID, id)
	}

	enc, err := encrypt.NewEncrypter([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("encrypter: %v", err)
	}
	taskRepo := repository.NewTaskRepository(pool)
	warmupRepo := repository.NewWarmupRepository(pool)
	emailRepo := repository.NewEmailRepostory(handle, enc)
	campaignRepo := repository.NewCampaignRepostory(handle)
	f.svc = &tasksService{
		tasksClient:  noopTaskScheduler{},
		scheduler:    scheduler.NewSchedulerService(taskRepo, warmupRepo, nil, emailRepo, campaignRepo, nil, nil),
		emailSender:  f.sender,
		warmupHealth: warmupapp.NewService(warmupRepo),
		taskRepo:     taskRepo,
		warmupRepo:   warmupRepo,
		emailRepo:    emailRepo,
		campaignRepo: campaignRepo,
	}
	return f
}

func (f *capFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
	}
}

// sentToday records n warmup sends already completed against today.
func (f *capFixture) sentToday(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		f.exec(t, `INSERT INTO tasks (id, task_type, email_account_id, status, message_id, scheduled_at, completed_at)
		      VALUES ($1, 'warmup', $2, 'completed', $3, now(), now())`,
			uuid.New(), f.mailbox, "<sent-"+uuid.New().String()+"@test.local>")
	}
}

func (f *capFixture) placement(t *testing.T) {
	t.Helper()
	f.exec(t, `INSERT INTO warmup_spam_reports (id, reporter_account_id, reported_account_id, message_id, report_type, created_at)
	      VALUES (gen_random_uuid(), $1, $1, $2, 'spam_placement', now())`,
		f.mailbox, "msg-"+uuid.New().String())
}

// pending places the mailbox's one warmup wakeup at the given time and
// returns its id.
func (f *capFixture) pending(t *testing.T, at time.Time) uuid.UUID {
	t.Helper()
	if err := f.svc.createWarmupTask(context.Background(), f.mailbox, at); err != nil {
		t.Fatalf("create pending warmup task: %v", err)
	}
	id, _ := f.pendingTask(t)
	return id
}

// pendingTask is the mailbox's pending wakeup with its aim, or uuid.Nil.
func (f *capFixture) pendingTask(t *testing.T) (uuid.UUID, *uuid.UUID) {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `
		SELECT t.id, wt.target_account_id
		FROM tasks t LEFT JOIN warmup_tasks wt ON wt.task_id = t.id
		WHERE t.email_account_id = $1 AND t.task_type = 'warmup' AND t.status = 'pending'`, f.mailbox)
	if err != nil {
		t.Fatalf("pending tasks: %v", err)
	}
	defer rows.Close()
	var id uuid.UUID
	var target *uuid.UUID
	n := 0
	for rows.Next() {
		if err := rows.Scan(&id, &target); err != nil {
			t.Fatalf("scan: %v", err)
		}
		n++
	}
	if n > 1 {
		t.Fatalf("%d pending warmup tasks; the chain must hold exactly one", n)
	}
	return id, target
}

func (f *capFixture) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM tasks WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("task status: %v", err)
	}
	return status
}

func (f *capFixture) run(t *testing.T, id uuid.UUID) {
	t.Helper()
	if xerr := f.svc.HandleEmailTask(&proto.ProcessTask{TaskId: id.String()}); xerr != nil {
		t.Fatalf("HandleEmailTask: %v", xerr.Message)
	}
}

func (f *capFixture) budget(t *testing.T) scheduler.WarmupBudget {
	t.Helper()
	b, err := f.svc.scheduler.WarmupDailyBudget(context.Background(), f.mailbox)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	return b
}

// budgetFailingScheduler is the real scheduler with the send-time budget read
// broken, which is what a database blip looks like at that moment.
type budgetFailingScheduler struct {
	scheduler.SchedulerService
	err error
}

func (s budgetFailingScheduler) WarmupDailyBudget(context.Context, uuid.UUID) (scheduler.WarmupBudget, error) {
	return scheduler.WarmupBudget{}, s.err
}

// Not knowing the day's count is exactly when a send must not go out: failing
// open there would reopen the bug whenever the database is struggling. A failed
// campaign read surfaces as "not warming", so that sentinel holds the send too.
func TestLiveWarmupUnreadableBudgetHoldsTheSendForRetry(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"read failed", errors.New("budget read failed")},
		{"campaign read failed and reported not warming", scheduler.ErrWarmupNotEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCapFixture(t)
			f.sentToday(t, 8)
			task := f.pending(t, time.Now())
			f.svc.scheduler = budgetFailingScheduler{SchedulerService: f.svc.scheduler, err: tc.err}

			if xerr := f.svc.HandleEmailTask(&proto.ProcessTask{TaskId: task.String()}); xerr == nil {
				t.Fatal("an unreadable budget was reported as success; the task would be acknowledged and never retried")
			}
			if f.sender.sent != 0 {
				t.Fatalf("%d send(s) dispatched without knowing today's count", f.sender.sent)
			}
			if got := f.status(t, task); got != "pending" {
				t.Fatalf("task status = %q, want pending so the retry picks it up", got)
			}
		})
	}
}

func TestLiveWarmupPendingSendRespectsTargetCutAfterScheduling(t *testing.T) {
	f := newCapFixture(t)
	f.sentToday(t, 8)
	if b := f.budget(t); b.Target != 10 || b.Sent != 8 || b.Reached() {
		t.Fatalf("before the placement: budget %+v, want target 10 with 8 sent", b)
	}
	task := f.pending(t, time.Now())

	// The placement lands while the send is waiting; the day is now 8.
	f.placement(t)
	if b := f.budget(t); b.Target != 8 || !b.Reached() {
		t.Fatalf("after the placement: budget %+v, want the cut target of 8, reached", b)
	}

	f.run(t, task)

	if f.sender.sent != 0 {
		t.Fatalf("target was cut to 8 but the pending send still went out: %d send(s) dispatched", f.sender.sent)
	}
	if got := f.status(t, task); got != "skipped_daily_limit" {
		t.Fatalf("task status = %q, want skipped_daily_limit", got)
	}
	next, _ := f.pendingTask(t)
	if next == uuid.Nil {
		t.Fatal("the chain was not rescheduled; the mailbox would never warm again")
	}
	var at time.Time
	if err := f.pool.QueryRow(context.Background(), `SELECT scheduled_at FROM tasks WHERE id = $1`, next).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if at.Before(time.Now().Add(time.Hour)) {
		t.Fatalf("rescheduled for %s; a spent day parks the chain at the next opening, not now", at)
	}
}

func TestLiveWarmupSendUnderTargetStillGoesOut(t *testing.T) {
	f := newCapFixture(t)
	f.sentToday(t, 8)
	task := f.pending(t, time.Now())

	f.run(t, task)

	if f.sender.sent != 1 {
		t.Fatalf("%d send(s) dispatched, want 1: the gate must only hold a spent day", f.sender.sent)
	}
	if got := f.status(t, task); got != "completed" {
		t.Fatalf("task status = %q, want completed", got)
	}
	if b := f.budget(t); b.Sent != 9 {
		t.Fatalf("sent today = %d after the send, want 9", b.Sent)
	}
}

// A reply-back re-points the pending send and pulls it earlier, including
// out of tomorrow into a day that is already spent. The cap holds it, and the
// aim survives so the answer goes out at the next opening instead of being
// dropped.
func TestLiveWarmupReplyBackPulledIntoASpentDayKeepsItsAim(t *testing.T) {
	f := newCapFixture(t)
	f.sentToday(t, 10)
	task := f.pending(t, time.Now().Add(6*time.Hour))
	writer := f.partners[0]
	moved, err := f.svc.taskRepo.DirectPendingWarmupTask(context.Background(), f.mailbox, writer, time.Now())
	if err != nil || !moved {
		t.Fatalf("direct pending task: moved=%v err=%v", moved, err)
	}

	f.run(t, task)

	if f.sender.sent != 0 {
		t.Fatalf("%d send(s) dispatched over a spent day", f.sender.sent)
	}
	if got := f.status(t, task); got != "skipped_daily_limit" {
		t.Fatalf("task status = %q, want skipped_daily_limit", got)
	}
	next, target := f.pendingTask(t)
	if next == uuid.Nil {
		t.Fatal("no successor task")
	}
	if target == nil || *target != writer {
		t.Fatalf("successor aimed at %v, want the reply-back's writer %s", target, writer)
	}
}

// A suspended workspace's send is held under a status the enum carries, so the
// row leaves pending. It used to write a value the enum lacked, the write
// failed silently, and the dispatcher fired the task again every tick.
func TestLiveWarmupSuspendedWorkspaceMarksTheTask(t *testing.T) {
	f := newCapFixture(t)
	handle := liveCampaignDB(t)
	f.exec(t, `UPDATE organizations SET risk_state = 'suspended' WHERE id = $1`, f.org)
	f.svc.orgRiskRepo = repository.NewOrgRiskRepository(handle)
	task := f.pending(t, time.Now())

	f.run(t, task)

	if f.sender.sent != 0 {
		t.Fatalf("%d send(s) dispatched from a suspended workspace", f.sender.sent)
	}
	if got := f.status(t, task); got != "skipped_org_suspended" {
		t.Fatalf("task status = %q, want skipped_org_suspended", got)
	}
}

// statusFailingRepo is the real repository with one status write refused,
// which is what a database blip looks like at that write.
type statusFailingRepo struct {
	repository.TaskRepository
	refuse string
}

func (r statusFailingRepo) UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, status string) error {
	if status == r.refuse {
		return errors.New("status write failed")
	}
	return r.TaskRepository.UpdateTaskStatus(ctx, taskID, status)
}

// A hold whose status write fails must not be reported as handled: the row
// stays pending, and acknowledging it would leave it blocking the successor
// until the overdue sweep. Discarding this error is what hid a status the enum
// did not carry for months.
func TestLiveWarmupHoldIsNotAcknowledgedUntilTheTaskIsMarked(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  string
		arrange func(t *testing.T, f *capFixture)
	}{
		{"daily target reached", "skipped_daily_limit", func(t *testing.T, f *capFixture) {
			f.sentToday(t, 10)
		}},
		{"workspace suspended", "skipped_org_suspended", func(t *testing.T, f *capFixture) {
			f.exec(t, `UPDATE organizations SET risk_state = 'suspended' WHERE id = $1`, f.org)
			f.svc.orgRiskRepo = repository.NewOrgRiskRepository(liveCampaignDB(t))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCapFixture(t)
			tc.arrange(t, f)
			task := f.pending(t, time.Now())
			f.svc.taskRepo = statusFailingRepo{TaskRepository: f.svc.taskRepo, refuse: tc.status}

			if xerr := f.svc.HandleEmailTask(&proto.ProcessTask{TaskId: task.String()}); xerr == nil {
				t.Fatal("the hold was reported as handled although its status write failed")
			}
			if f.sender.sent != 0 {
				t.Fatalf("%d send(s) dispatched", f.sender.sent)
			}
			if got := f.status(t, task); got != "pending" {
				t.Fatalf("task status = %q, want pending so the retry picks it up", got)
			}
		})
	}
}
