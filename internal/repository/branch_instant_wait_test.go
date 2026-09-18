package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

// The wait a routed step sits behind is the TARGET step's own wait_after, but
// an instant branch already saw its signal when it matched, so its flag has to
// win over that delay (issue #583). These cases build the router directly
// rather than through Postgres: the route() decision is the sequencing layer
// under test, and it is the same call FindRoutedPairs makes per row.
func TestRoutedWaitHonorsTheBranchInstantFlag(t *testing.T) {
	openedWithinADay := []models.BranchCondition{{Field: "opened", Operator: "within_days", Value: intPtr(1)}}
	instant, optedOut := true, false

	cases := []struct {
		name            string
		conditions      []models.BranchCondition
		instant         *bool
		targetWaitAfter int
		wantWait        time.Duration
	}{
		{"instant branch drops the target wait", openedWithinADay, &instant, 10, 0},
		{"instant defaults on when the flag is omitted", openedWithinADay, nil, 10, 0},
		{"opted-out branch keeps the target wait", openedWithinADay, &optedOut, 10, 10 * 24 * time.Hour},
		{"instant branch with no wait left to drop", openedWithinADay, &instant, 0, 0},
		// A catch-all ("otherwise") is not an instant-capable signal branch, so
		// a stray instant flag on it must not wipe the target's delay.
		{"branch without an instant-capable condition keeps its wait", nil, &instant, 10, 10 * 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, target := uuid.New(), uuid.New()
			sent := time.Now().Add(-2 * time.Hour)
			cr := &campaignRouter{
				steps: []routeStep{
					{id: source, kind: "email", bc: models.BranchConditions{Branches: []models.Branch{{
						BranchID:         "b1",
						TargetSequenceID: &target,
						Conditions:       tc.conditions,
						Instant:          tc.instant,
					}}}},
					{id: target, kind: "email", waitAfter: tc.targetWaitAfter},
				},
				idxByID:        map[uuid.UUID]int{source: 0, target: 1},
				entry:          source,
				now:            time.Now(),
				dueBy:          time.Now().Add(time.Minute),
				replyFlowSteps: map[uuid.UUID]bool{},
			}
			opened := sent.Add(30 * time.Minute)

			got := cr.route(uuid.New(), uuid.New(), routeInput{
				lastSeq: &source, sentAt: &sent, openedAt: &opened,
			})

			if got.Target == nil || *got.Target != target {
				t.Fatalf("target = %v, want %s", got.Target, target)
			}
			if got.DueAt == nil {
				t.Fatalf("DueAt is nil, want %s after the last send", tc.wantWait)
			}
			want := sent.Add(tc.wantWait)
			if diff := got.DueAt.Sub(want); diff > time.Second || diff < -time.Second {
				t.Fatalf("DueAt = %s, want %s (target wait_after %d)", got.DueAt, want, tc.targetWaitAfter)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
