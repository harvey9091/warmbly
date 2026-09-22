package advanced

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type bounceTaskRepo struct {
	repository.TaskRepository
	task          *repository.Task
	campaignTasks int
}

func (r *bounceTaskRepo) GetTaskByMessageID(context.Context, string) (*repository.Task, error) {
	return r.task, nil
}

func (r *bounceTaskRepo) GetCampaignTask(context.Context, uuid.UUID) (*repository.CampaignTask, error) {
	r.campaignTasks++
	return nil, nil
}

// bounceEmailRepo fails the lookup that comes AFTER the warmup gate, so a test
// can tell "refused as warmup" from "got further and then stopped".
type bounceEmailRepo struct {
	repository.EmailRepository
	reads int
}

func (r *bounceEmailRepo) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	r.reads++
	return nil, errx.ErrNotFound
}

// A warmup send's NDR resolves to a task exactly like a campaign send's, because
// warmup stamps tasks.message_id too. Attributing it suppressed a pool
// partner's address in the customer's list and recorded a bounce against their
// deliverability, where it fed the breaker.
func TestRecordInboundBounceRefusesWarmupSends(t *testing.T) {
	mailbox := uuid.New()
	tasks := &bounceTaskRepo{task: &repository.Task{
		ID:             uuid.New(),
		TaskType:       models.TaskTypeWarmup,
		EmailAccountID: mailbox,
		MessageID:      "<warm-1@sender.test>",
	}}
	emails := &bounceEmailRepo{}
	s := &service{taskRepo: tasks, emailRepo: emails}

	if err := s.RecordInboundBounce(context.Background(), mailbox, "warm-1@sender.test", "partner@pool.test", "550 mailbox unavailable"); err != nil {
		t.Fatalf("a warmup NDR should be dropped quietly, got %v", err)
	}
	if emails.reads != 0 {
		t.Fatal("a warmup NDR was carried past the gate and into deliverability ingest")
	}
	if tasks.campaignTasks != 0 {
		t.Fatal("a warmup NDR should not be looked up as a campaign send")
	}
}

// The same NDR for a real send still has to be attributed, or the gate has
// bought silence at the cost of campaign bounce tracking.
func TestRecordInboundBounceStillAttributesRealSends(t *testing.T) {
	mailbox := uuid.New()
	tasks := &bounceTaskRepo{task: &repository.Task{
		ID:             uuid.New(),
		TaskType:       "campaign",
		EmailAccountID: mailbox,
		MessageID:      "<camp-1@sender.test>",
	}}
	emails := &bounceEmailRepo{}
	s := &service{taskRepo: tasks, emailRepo: emails}

	// The mailbox lookup refuses, so ingest stops there; reaching it at all is
	// what this asserts.
	_ = s.RecordInboundBounce(context.Background(), mailbox, "camp-1@sender.test", "lead@prospect.test", "550 no such user")
	if emails.reads == 0 {
		t.Fatal("a campaign NDR was dropped by the warmup gate")
	}
}
