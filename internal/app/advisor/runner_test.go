package advisor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

type blockingEvaluator struct {
	Service
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (b *blockingEvaluator) Evaluate(ctx context.Context, orgID uuid.UUID, trigger string) (*models.AdvisorSummary, error) {
	if b.calls.Add(1) == 1 {
		close(b.started)
		<-b.release
	}
	return &models.AdvisorSummary{}, nil
}

func waitIdle(t *testing.T, orgID uuid.UUID) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		refreshMu.Lock()
		_, busy := refreshing[orgID]
		refreshMu.Unlock()
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("refresh never finished")
}

func TestRefreshNowDuringRunEvaluatesOnceMore(t *testing.T) {
	orgID := uuid.New()
	svc := &blockingEvaluator{started: make(chan struct{}), release: make(chan struct{})}

	RefreshNow(svc, orgID, "change")
	<-svc.started
	// Three changes land while the first pass is still reading.
	RefreshNow(svc, orgID, "change")
	RefreshNow(svc, orgID, "change")
	RefreshNow(svc, orgID, "change")
	close(svc.release)
	waitIdle(t, orgID)

	if got := svc.calls.Load(); got != 2 {
		t.Fatalf("evaluations = %d, want 2 (the running pass plus one that sees the changes)", got)
	}
}

func TestStaleRefreshDuringRunDoesNotRepeat(t *testing.T) {
	orgID := uuid.New()
	svc := &blockingEvaluator{started: make(chan struct{}), release: make(chan struct{})}

	startRefresh(svc, orgID, "read", false)
	<-svc.started
	startRefresh(svc, orgID, "read", false)
	close(svc.release)
	waitIdle(t, orgID)

	if got := svc.calls.Load(); got != 1 {
		t.Fatalf("evaluations = %d, want 1", got)
	}
}
