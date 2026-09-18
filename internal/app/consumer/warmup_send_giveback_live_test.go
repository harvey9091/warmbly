package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// A warmup send is counted at dispatch, before the worker has done anything,
// and the scheduler's cap counts completed warmup tasks. HandleEmailFailed had
// no warmup branch, so a refused send stopped counting against the cap while
// the reported number kept it forever: the mailbox sent one extra message past
// its target per failure, which is how a day's target of 10 reached 21 with
// nothing delivered at all (#574).
//
//	WARMBLY_TEST_DB=postgres://... go test ./internal/app/consumer/ -run LiveWarmupSend -v
func TestLiveWarmupSendFailureGivesTheDayBack(t *testing.T) {
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB not set")
	}
	ctx := context.Background()
	handle, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { handle.Pool.Close() })

	f := newSendResultFixture(t, handle)
	s := &JobsService{
		TaskRepo:   repository.NewTaskRepository(handle.Pool),
		WarmupRepo: repository.NewWarmupRepository(handle.Pool),
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := handle.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := handle.Pool.Exec(context.Background(),
			`DELETE FROM warmup_statistics WHERE email_account_id = $1`, f.mailbox); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	// One tick as the control plane runs it: the task is stamped completed and
	// the day's two counters are taken, then the worker refuses the send.
	day := time.Now()
	taskID := uuid.New()
	exec(`INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	      VALUES ($1, 'warmup', $2, 'completed', '', $3)`, taskID, f.mailbox, day)
	exec(`INSERT INTO warmup_tokens (token, task_id, sender_account_id, recipient_account_id, conversation_turn)
	      VALUES (gen_random_uuid(), $1, $2, $2, 1)`, taskID, f.mailbox)
	if err := s.WarmupRepo.IncrementDailyCount(ctx, f.mailbox, day); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if err := s.WarmupRepo.IncrementReplyCount(ctx, f.mailbox, day); err != nil {
		t.Fatalf("increment reply: %v", err)
	}

	read := func() (sent, replied int) {
		t.Helper()
		if err := handle.Pool.QueryRow(ctx, `SELECT emails_sent, emails_replied FROM warmup_statistics
		    WHERE email_account_id = $1 AND date = DATE($2)`, f.mailbox, day).Scan(&sent, &replied); err != nil {
			t.Fatalf("read statistics: %v", err)
		}
		return sent, replied
	}
	if sent, replied := read(); sent != 1 || replied != 1 {
		t.Fatalf("precondition: sent=%d replied=%d, want the dispatch counted 1/1", sent, replied)
	}

	if err := s.HandleEmailFailed(ctx, models.SendEmailResult{
		TaskID: taskID, Success: false,
		Error: &models.EmailSendError{Code: "SERVER_UNREACHABLE", Message: "the connection to the mail server could not be established"},
	}); err != nil {
		t.Fatalf("handle failed: %v", err)
	}

	var status string
	if err := handle.Pool.QueryRow(ctx, `SELECT status FROM tasks WHERE id = $1`, taskID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("task status = %q, want failed", status)
	}
	if sent, replied := read(); sent != 0 || replied != 0 {
		t.Fatalf("after the worker refused the send: sent=%d replied=%d, want 0/0", sent, replied)
	}

	// The cap the scheduler enforces reads completed warmup tasks, so the two
	// numbers now agree instead of drifting apart by one per failure.
	capCount, err := s.TaskRepo.CountWarmupEmailsSentToday(ctx, f.mailbox)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	sent, _ := read()
	if capCount != sent {
		t.Fatalf("the cap counts %d and the reported number %d; they must be the same send", capCount, sent)
	}
}

// A second delivery of the same result must not double-refund the day.
func TestLiveWarmupSendGiveBackIsIdempotent(t *testing.T) {
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB not set")
	}
	ctx := context.Background()
	handle, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { handle.Pool.Close() })

	f := newSendResultFixture(t, handle)
	s := &JobsService{
		TaskRepo:   repository.NewTaskRepository(handle.Pool),
		WarmupRepo: repository.NewWarmupRepository(handle.Pool),
	}
	t.Cleanup(func() {
		if _, err := handle.Pool.Exec(context.Background(),
			`DELETE FROM warmup_statistics WHERE email_account_id = $1`, f.mailbox); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	day := time.Now()
	taskID := uuid.New()
	if _, err := handle.Pool.Exec(ctx, `INSERT INTO tasks (id, task_type, email_account_id, status, message_id, completed_at)
	      VALUES ($1, 'warmup', $2, 'completed', '', $3)`, taskID, f.mailbox, day); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.WarmupRepo.IncrementDailyCount(ctx, f.mailbox, day); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	result := models.SendEmailResult{
		TaskID: taskID, Success: false,
		Error: &models.EmailSendError{Code: "SERVER_UNREACHABLE", Message: "unreachable"},
	}
	for i := 0; i < 3; i++ {
		if err := s.HandleEmailFailed(ctx, result); err != nil {
			t.Fatalf("handle failed (%d): %v", i, err)
		}
	}

	var sent int
	if err := handle.Pool.QueryRow(ctx, `SELECT emails_sent FROM warmup_statistics
	    WHERE email_account_id = $1 AND date = DATE($2)`, f.mailbox, day).Scan(&sent); err != nil {
		t.Fatalf("read statistics: %v", err)
	}
	if sent != 2 {
		t.Fatalf("emails_sent = %d, want 2: three deliveries of one failure give one send back", sent)
	}
}
