package advisor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/jobrun"
	"github.com/warmbly/warmbly/internal/repository"
)

// Runner keeps every org's findings current in the background.
//
// The pacing is deliberately unhurried. Advice about a 30-day complaint rate
// does not change minute to minute, and an advisor that re-evaluates
// constantly costs database load without ever telling anyone something new.
// What it does need is to never go stale enough that a member opens a page and
// sees advice about a mailbox they fixed yesterday, which is what the
// event-driven refresh on the read path is for.
type Runner struct {
	Repo    repository.AdvisorRepository
	Service Service

	// Interval is how often the loop wakes. Each tick evaluates a bounded
	// batch, so a large install spreads its evaluations across ticks rather
	// than stampeding.
	Interval time.Duration
	// Staleness is how old an org's last run must be before it is re-evaluated.
	Staleness time.Duration
	// BatchSize bounds how many orgs one tick evaluates.
	BatchSize int
}

// Default pacing: wake every five minutes, re-evaluate an org at most every
// six hours, and never do more than twenty orgs at once.
const (
	defaultInterval  = 5 * time.Minute
	defaultStaleness = 6 * time.Hour
	defaultBatchSize = 20
)

// Run drives the loop until the context is cancelled.
func (r *Runner) Run(ctx context.Context) {
	if r.Interval <= 0 {
		r.Interval = defaultInterval
	}
	if r.Staleness <= 0 {
		r.Staleness = defaultStaleness
	}
	if r.BatchSize <= 0 {
		r.BatchSize = defaultBatchSize
	}
	jobrun.Loop(ctx, "advisor_sweep", r.Interval, false, func(ctx context.Context) error {
		r.sweep(ctx)
		return nil
	})
}

func (r *Runner) sweep(ctx context.Context) {
	// Snoozes first: a snooze that expired should be visible on the next page
	// load, not on the next time that org happens to be re-evaluated.
	if n, err := r.Repo.ExpireSnoozes(ctx); err != nil {
		log.Printf("advisor: expire snoozes: %v", err)
	} else if n > 0 {
		log.Printf("advisor: reopened %d snoozed findings", n)
	}

	orgs, err := r.Repo.ListOrgsDue(ctx, r.Staleness, r.BatchSize)
	if err != nil {
		log.Printf("advisor: list due orgs: %v", err)
		return
	}

	for _, orgID := range orgs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// One org's failure must not stop the batch: a malformed campaign in
		// one workspace should not cost every other workspace its advice.
		if _, err := r.Service.Evaluate(ctx, orgID, "schedule"); err != nil {
			log.Printf("advisor: evaluate org %s: %v", orgID, err)
		}
	}
}

// refreshing dedupes concurrent refreshes for one org, so three open tabs (or a
// page that reads the summary and the list together) cost one evaluation
// rather than three. The value records that a forced refresh arrived mid-run,
// whose change the running pass may have read too early to see.
var (
	refreshMu  sync.Mutex
	refreshing = map[uuid.UUID]bool{}
)

// refreshTimeout bounds a detached refresh. An evaluation that has not finished
// in this long is stuck on something, and holding the goroutine open does not
// help anyone.
const refreshTimeout = 2 * time.Minute

// RefreshIfStale kicks off an evaluation when an org's findings are older than
// maxAge. It never blocks the caller: the request that triggered it returns the
// data currently stored, and the finished evaluation reaches every open
// dashboard through the audit spine, which is the same path a teammate's change
// takes. Doing it inline instead would put a handful of queries plus up to a
// dozen model calls in front of a page load, to deliver a result the client is
// about to receive anyway.
func RefreshIfStale(ctx context.Context, repo repository.AdvisorRepository, svc Service, orgID uuid.UUID, maxAge time.Duration) {
	last, err := repo.LastRunAt(ctx, orgID)
	if err != nil {
		return
	}
	if last != nil && time.Since(*last) < maxAge {
		return
	}
	startRefresh(svc, orgID, "read", false)
}

// RefreshNow re-evaluates an org after a change the advisor reads (a mailbox
// removed or reconfigured), whatever the age of its findings. It never blocks.
func RefreshNow(svc Service, orgID uuid.UUID, trigger string) {
	startRefresh(svc, orgID, trigger, true)
}

func startRefresh(svc Service, orgID uuid.UUID, trigger string, forced bool) {
	refreshMu.Lock()
	if _, busy := refreshing[orgID]; busy {
		if forced {
			refreshing[orgID] = true
		}
		refreshMu.Unlock()
		return
	}
	refreshing[orgID] = false
	refreshMu.Unlock()

	// Detached from the request context on purpose: the user navigating away
	// must not abort an evaluation that is already running and about to publish
	// its result to their teammates.
	go func() {
		for {
			bg, cancel := context.WithTimeout(context.Background(), refreshTimeout)
			if _, err := svc.Evaluate(bg, orgID, trigger); err != nil {
				log.Printf("advisor: background refresh for org %s: %v", orgID, err)
			}
			cancel()

			refreshMu.Lock()
			if !refreshing[orgID] {
				delete(refreshing, orgID)
				refreshMu.Unlock()
				return
			}
			refreshing[orgID] = false
			refreshMu.Unlock()
		}
	}()
}
