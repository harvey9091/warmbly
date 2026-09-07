package jobs

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/observability/errs"

	emailverifyapp "github.com/warmbly/warmbly/internal/app/emailverify"
)

// DeliveryEvidenceJob turns campaign sends that never bounced into
// verification evidence: a delivery the recipient's server kept is the
// strongest proof the mailbox exists, and it costs nothing to observe.
type DeliveryEvidenceJob struct {
	evidence *emailverifyapp.Evidence
	interval time.Duration
	batch    int
}

func NewDeliveryEvidenceJob(evidence *emailverifyapp.Evidence, interval time.Duration, batch int) *DeliveryEvidenceJob {
	if batch <= 0 {
		batch = 2000
	}
	return &DeliveryEvidenceJob{evidence: evidence, interval: interval, batch: batch}
}

// Start runs the job on its interval until ctx ends. A full batch repeats
// at once so a backlog drains.
func (j *DeliveryEvidenceJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
		for {
			n, err := j.evidence.CreditCleanDeliveries(ctx, j.batch)
			if err != nil {
				errs.CaptureException(err)
				break
			}
			if n < j.batch || ctx.Err() != nil {
				break
			}
		}
	}
}
