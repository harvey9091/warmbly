package inboxtag

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/repository"
)

func candidates(n int) []repository.BackfillCandidate {
	out := make([]repository.BackfillCandidate, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, repository.BackfillCandidate{
			EmailAccountID: uuid.New(),
			MessageID:      "<old-" + string(rune('a'+i)) + "@example.com>",
			ThreadID:       "thread-1",
			Subject:        "Re: Collaboration idea",
			BodyText:       "Yes let's do it, happy to go ahead with the partnership this week.",
			FromAddr:       "Jane <jane@example.com>",
			InternalDate:   time.Now().Add(-time.Duration(i) * time.Hour),
		})
	}
	return out
}

// The point of a backfill: the inbox you want sorted is the one already sitting
// there, so a classifier that only ever sees new arrivals is useless on the day
// it is switched on.
func TestBackfillClassifiesHistory(t *testing.T) {
	_, responses := loadFixtures(t)
	asker := &countingAsker{resp: responses["agreed_partnership"]}
	repo := &fakeRepo{untagged: candidates(5)}
	svc := newService(t, asker, repo)

	p, err := svc.Backfill(context.Background(), uuid.New(), BackfillOptions{Since: time.Now().Add(-30 * 24 * time.Hour)})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if p.Classified != 5 {
		t.Errorf("classified %d of 5", p.Classified)
	}
	if asker.calls != 5 {
		t.Errorf("made %d calls for 5 messages, want one each", asker.calls)
	}
	if len(repo.saved) != 5 {
		t.Errorf("saved %d rows, want 5", len(repo.saved))
	}
}

// A dry run is how you find out what a backfill would cost before paying for
// it, so it must call nothing and write nothing.
func TestBackfillDryRunCallsNothing(t *testing.T) {
	_, responses := loadFixtures(t)
	asker := &countingAsker{resp: responses["agreed_partnership"]}
	repo := &fakeRepo{untagged: candidates(4)}
	svc := newService(t, asker, repo)

	p, err := svc.Backfill(context.Background(), uuid.New(), BackfillOptions{DryRun: true})
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if asker.calls != 0 {
		t.Errorf("dry run made %d calls", asker.calls)
	}
	if len(repo.saved) != 0 {
		t.Errorf("dry run saved %d rows", len(repo.saved))
	}
	if p.Considered != 4 {
		t.Errorf("dry run considered %d, want 4 — it still has to report the size", p.Considered)
	}
}

// Re-running must cost nothing: each message is idempotent on its Message-ID,
// which is what makes an interrupted run resumable rather than a restart.
func TestBackfillIsResumable(t *testing.T) {
	_, responses := loadFixtures(t)
	asker := &countingAsker{resp: responses["agreed_partnership"]}
	repo := &fakeRepo{untagged: candidates(3)}
	svc := newService(t, asker, repo)
	orgID := uuid.New()

	if _, err := svc.Backfill(context.Background(), orgID, BackfillOptions{}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := asker.calls

	// The same candidates offered again: the real query excludes tagged rows,
	// but the service must not depend on that to stay idempotent.
	if _, err := svc.Backfill(context.Background(), orgID, BackfillOptions{}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if asker.calls != first {
		t.Errorf("second run made %d more calls; a re-run must cost nothing", asker.calls-first)
	}
}

// Cancelling stops cleanly and keeps what was already done. A backfill over a
// large mailbox is long enough that someone will Ctrl-C it.
func TestBackfillStopsOnCancel(t *testing.T) {
	_, responses := loadFixtures(t)
	asker := &countingAsker{resp: responses["agreed_partnership"]}
	repo := &fakeRepo{untagged: candidates(10)}
	svc := newService(t, asker, repo)

	ctx, cancel := context.WithCancel(context.Background())
	opts := BackfillOptions{
		OnProgress: func(p BackfillProgress, _ string) {
			if p.Considered == 3 {
				cancel()
			}
		},
	}
	_, err := svc.Backfill(ctx, uuid.New(), opts)
	if err == nil {
		t.Fatal("expected the cancelled context to be reported")
	}
	if len(repo.saved) == 0 {
		t.Error("cancelling threw away work that was already done")
	}
	if len(repo.saved) >= 10 {
		t.Errorf("cancel did not stop the run: %d of 10 processed", len(repo.saved))
	}
}

// A disabled instance has nothing to backfill with, and must say so rather than
// silently doing nothing.
func TestBackfillRefusedWhenDisabled(t *testing.T) {
	svc := NewService(&countingAsker{}, &fakeRepo{}, nil, nil, false)
	if _, err := svc.Backfill(context.Background(), uuid.New(), BackfillOptions{}); err == nil {
		t.Fatal("expected an error when the feature is off")
	}
}
