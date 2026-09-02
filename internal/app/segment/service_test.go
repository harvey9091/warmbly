package segment

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// syncRepo is a SegmentRepository that only answers LinkedCampaigns, blocking
// there until released so a second request provably arrives mid-pass. Every
// other method is left to the embedded nil interface: this test must not reach
// them, and a panic is a clearer failure than a silent zero value.
type syncRepo struct {
	repository.SegmentRepository
	entered chan struct{}
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func (r *syncRepo) LinkedCampaigns(context.Context, *uuid.UUID) ([]models.LinkedCampaign, *errx.Error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	r.entered <- struct{}{}
	<-r.release
	return nil, nil
}

func (r *syncRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitFor(t *testing.T, c chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// A sync requested while a pass is running must run again afterwards. The
// import path depends on it: ImportCommit's chunked Add starts a pass, then
// writes segment membership, then asks for a sync. Dropping that request
// leaves freshly pinned contacts out of their linked campaigns until the
// periodic sweep.
func TestSyncOrgLinkedCampaignsCoalescesMidPassRequest(t *testing.T) {
	repo := &syncRepo{entered: make(chan struct{}, 4), release: make(chan struct{})}
	svc := NewService(repo, nil).(*service)
	org := uuid.New()
	ctx := context.Background()

	svc.SyncOrgLinkedCampaigns(ctx, org)
	waitFor(t, repo.entered, "the first pass to start")

	// Two requests land while the first pass is blocked: both are folded into
	// one follow-up, not stacked and not lost.
	svc.SyncOrgLinkedCampaigns(ctx, org)
	svc.SyncOrgLinkedCampaigns(ctx, org)
	if got := repo.callCount(); got != 1 {
		t.Fatalf("passes started while one was running = %d, want 1", got)
	}

	repo.release <- struct{}{}
	waitFor(t, repo.entered, "the coalesced follow-up pass")
	repo.release <- struct{}{}

	// The follow-up drains the flag, so nothing runs a third time.
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case <-repo.entered:
			t.Fatalf("a third pass ran; the follow-up flag was not cleared")
		case <-deadline:
			if got := repo.callCount(); got != 2 {
				t.Fatalf("total passes = %d, want 2", got)
			}
			svc.syncMu.Lock()
			_, still := svc.orgSync[org]
			svc.syncMu.Unlock()
			if still {
				t.Errorf("org sync state was not released once idle")
			}
			return
		}
	}
}

// Back-to-back requests with no overlap each get their own pass.
func TestSyncOrgLinkedCampaignsRunsAgainAfterIdle(t *testing.T) {
	repo := &syncRepo{entered: make(chan struct{}, 4), release: make(chan struct{})}
	svc := NewService(repo, nil).(*service)
	org := uuid.New()
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		svc.SyncOrgLinkedCampaigns(ctx, org)
		waitFor(t, repo.entered, "a pass to start")
		repo.release <- struct{}{}
		// Let the goroutine retire the state before asking again.
		for j := 0; j < 100; j++ {
			svc.syncMu.Lock()
			_, running := svc.orgSync[org]
			svc.syncMu.Unlock()
			if !running {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if got := repo.callCount(); got != 2 {
		t.Fatalf("passes = %d, want 2", got)
	}
}
