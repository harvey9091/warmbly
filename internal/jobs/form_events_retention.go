package jobs

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/repository"
)

// FormEventsRetentionJob prunes funnel events past the platform window; the
// forms analytics ranges top out at 90 days, so the fixed window keeps double
// coverage without a per-org setting.
type FormEventsRetentionJob struct {
	repo repository.FormEventRepository
}

func NewFormEventsRetentionJob(repo repository.FormEventRepository) *FormEventsRetentionJob {
	return &FormEventsRetentionJob{repo: repo}
}

func (j *FormEventsRetentionJob) Run(ctx context.Context) error {
	if j.repo == nil {
		return nil
	}
	before := time.Now().AddDate(0, 0, -config.FormEventsRetentionDays)
	if _, xerr := j.repo.PruneBefore(ctx, before); xerr != nil {
		return xerr
	}
	return nil
}

// Start runs the job once on boot and then on the interval until ctx ends.
func (j *FormEventsRetentionJob) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	_ = j.Run(ctx)
	for {
		select {
		case <-ticker.C:
			_ = j.Run(ctx)
		case <-ctx.Done():
			return
		}
	}
}
