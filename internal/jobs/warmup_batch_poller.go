package jobs

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/observability/errs"

	"github.com/warmbly/warmbly/internal/app/warmupcontent"
)

// WarmupBatchPoller reconciles in-flight OpenAI Batch API warmup-generation jobs:
// it polls each active batch, ingests what each finished one produced into the
// content bank, and marks empty ones failed. It is a thin scheduler around
// warmupcontent.Service.PollBatches; all policy lives in the service. Batches run
// async (up to a 24h window) so a coarse 5-minute tick is plenty.
type WarmupBatchPoller struct {
	svc      warmupcontent.Service
	interval time.Duration
	stopCh   chan struct{}
}

// NewWarmupBatchPoller creates the poller.
func NewWarmupBatchPoller(svc warmupcontent.Service, interval time.Duration) *WarmupBatchPoller {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &WarmupBatchPoller{
		svc:      svc,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Run performs one reconciliation pass. Safe to call frequently — PollBatches
// no-ops when generation is unconfigured or there are no active batch jobs.
func (p *WarmupBatchPoller) Run(ctx context.Context) error {
	if p.svc == nil || !p.svc.Enabled() {
		return nil
	}
	if err := p.svc.PollBatches(ctx); err != nil {
		errs.CaptureException(err)
		return err
	}
	return nil
}

// Start begins scheduled execution on the configured interval.
func (p *WarmupBatchPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := p.Run(ctx); err != nil {
				errs.CaptureException(err)
			}
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop halts scheduled execution.
func (p *WarmupBatchPoller) Stop() {
	close(p.stopCh)
}
