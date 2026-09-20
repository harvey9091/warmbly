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

// A reply_intent condition is a strict pair with "is" and one intent name,
// like ai_label, and it must never validate against another operator or an
// unbounded label.
func TestValidateBranchConditionsReplyIntent(t *testing.T) {
	cases := []struct {
		name string
		cond models.BranchCondition
		ok   bool
	}{
		{"is with an intent", models.BranchCondition{Field: "reply_intent", Operator: "is", Label: "wants_pricing"}, true},
		{"empty label", models.BranchCondition{Field: "reply_intent", Operator: "is", Label: ""}, false},
		{"uppercase label", models.BranchCondition{Field: "reply_intent", Operator: "is", Label: "Agreed"}, false},
		{"label too long", models.BranchCondition{Field: "reply_intent", Operator: "is", Label: "a_very_long_intent_name_that_runs_past_the_cap"}, false},
		{"wrong operator", models.BranchCondition{Field: "reply_intent", Operator: "ever", Label: "agreed"}, false},
		{"is on an engagement field", models.BranchCondition{Field: "opened", Operator: "is", Label: "agreed"}, false},
		{"is still pairs with ai_label", models.BranchCondition{Field: "ai_label", Operator: "is", Label: "interested"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bc := &models.BranchConditions{Branches: []models.Branch{{BranchID: "b1", Conditions: []models.BranchCondition{tc.cond}}}}
			err := validateBranchConditions(bc)
			if (err == nil) != tc.ok {
				t.Fatalf("validateBranchConditions(%+v) err = %v, want ok=%v", tc.cond, err, tc.ok)
			}
		})
	}
}

// reply_intent decides immediately off the stored intent, case-insensitively,
// and an untagged reply matches nothing so it falls to the catch-all.
func TestConditionStateReplyIntent(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		label  string
		want   BranchState
	}{
		{"same intent", "wants_pricing", "wants_pricing", BranchMatch},
		{"case-insensitive", "Wants_Pricing", "wants_pricing", BranchMatch},
		{"different intent", "not_now", "wants_pricing", BranchNoMatch},
		{"never tagged", "", "wants_pricing", BranchNoMatch},
		{"never tagged and empty label", "", "", BranchNoMatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := &CampaignContactProgress{ContactID: uuid.New(), ReplyIntent: tc.stored}
			cond := models.BranchCondition{Field: "reply_intent", Operator: "is", Label: tc.label}
			got, at := conditionState(cond, prog, "b1", time.Now().Add(-time.Hour), time.Now())
			if got != tc.want {
				t.Fatalf("state = %v, want %v", got, tc.want)
			}
			if !at.IsZero() {
				t.Fatalf("recheck at = %s, want none: reply_intent has no window", at)
			}
		})
	}
}

// reply_intent fires on the reply event like the reply_* class fields, so an
// instant reply trigger routes on it the moment the reply lands.
func TestReplyIntentIsAnInstantReplyField(t *testing.T) {
	if !fieldBelongsToEvent("reply_intent", "reply") {
		t.Fatal("reply_intent should belong to the reply event")
	}
	if fieldBelongsToEvent("reply_intent", "open") || fieldBelongsToEvent("reply_intent", "click") {
		t.Fatal("reply_intent should not belong to open or click")
	}
	target := uuid.New()
	bc := &models.BranchConditions{Branches: []models.Branch{{
		BranchID:         "b1",
		TargetSequenceID: &target,
		Conditions:       []models.BranchCondition{{Field: "reply_intent", Operator: "is", Label: "agreed"}},
	}}}
	prog := &CampaignContactProgress{ContactID: uuid.New(), ReplyIntent: "agreed"}
	matched, got, instant := MatchInstantBranchTarget(bc, prog, "reply")
	if !matched || !instant || got == nil || *got != target {
		t.Fatalf("MatchInstantBranchTarget = (%v, %v, %v), want match on %s", matched, got, instant, target)
	}
	prog.ReplyIntent = "not_now"
	if matched, _, _ := MatchInstantBranchTarget(bc, prog, "reply"); matched {
		t.Fatal("a different intent must not match")
	}
}
