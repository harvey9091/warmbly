package inboxtag

import (
	"reflect"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func allOn() models.InboxTaggingSettings {
	return models.InboxTaggingSettings{
		HoldOnNotNow: true, NotNowHoldDays: 30, StopOnDeclined: true,
		TaskOnCallRequest: true, SuppressOnRemovalRequest: true,
	}
}

func humanReply(intent string, conf float64) Decision {
	return Decision{
		Kind: KindHumanReply, KindConfidence: 0.95, KindSource: "model",
		Intent: intent, IntentConfidence: conf,
		SignalStrength: map[string]float64{},
	}
}

// Every switch is off by default, so the zero settings plan nothing whatever
// the verdict says.
func TestPlanActionsDefaultsPlanNothing(t *testing.T) {
	d := humanReply(IntentNotInterested, 0.99)
	d.SignalStrength[SigRequestsRemoval] = 0.99
	if p := PlanActions(d, models.InboxTaggingSettings{}); !p.Empty() {
		t.Fatalf("default settings planned %v", p.Actions())
	}
}

func TestPlanActionsOnlyOnAConfidentHumanReply(t *testing.T) {
	cases := map[string]Decision{
		"bounce":       {Kind: KindBounceHard, KindConfidence: 1, Intent: IntentNotNow, IntentConfidence: 0.9},
		"needs review": {Kind: KindHumanReply, KindConfidence: 0.9, NeedsReview: true, Intent: IntentNotNow, IntentConfidence: 0.9},
		"kind floor":   {Kind: KindHumanReply, KindConfidence: 0.5, Intent: IntentNotNow, IntentConfidence: 0.9},
		"intent floor": humanReply(IntentNotNow, 0.5),
	}
	for name, d := range cases {
		if p := PlanActions(d, allOn()); !p.Empty() {
			t.Errorf("%s planned %v", name, p.Actions())
		}
	}
}

func TestPlanActionsByIntent(t *testing.T) {
	if p := PlanActions(humanReply(IntentNotNow, 0.8), allOn()); p.HoldDays != 30 || p.Stop {
		t.Fatalf("not_now: %+v", p)
	}
	for _, intent := range []string{IntentNotInterested, IntentWrongPerson} {
		if p := PlanActions(humanReply(intent, 0.8), allOn()); !p.Stop || p.HoldDays != 0 {
			t.Fatalf("%s: %+v", intent, p)
		}
	}
	if p := PlanActions(humanReply(IntentScheduling, 0.8), allOn()); p.Task == "" {
		t.Fatalf("scheduling should open a task: %+v", p)
	}
	d := humanReply(IntentWantsInfo, 0.8)
	d.Signals = []string{SigAsksForCall}
	if p := PlanActions(d, allOn()); p.Task == "" {
		t.Fatalf("asks_for_call should open a task: %+v", p)
	}
}

// The suppression reads the removal noul at Strong, not the opt_out intent,
// and outranks every other action on the same reply.
func TestPlanActionsSuppressNeedsStrongAndWins(t *testing.T) {
	d := humanReply(IntentOptOut, 0.95)
	d.SignalStrength[SigRequestsRemoval] = 0.7
	if p := PlanActions(d, allOn()); p.Suppress != "" {
		t.Fatalf("suppressed below Strong: %+v", p)
	}
	d.SignalStrength[SigRequestsRemoval] = 0.85
	d.Signals = []string{SigAsksForCall, SigRequestsRemoval}
	p := PlanActions(d, allOn())
	if p.Suppress == "" || p.Task != "" || p.HoldDays != 0 || p.Stop {
		t.Fatalf("suppression should stand alone: %+v", p)
	}
	if got := p.Actions(); !reflect.DeepEqual(got, []string{ActionSuppress}) {
		t.Fatalf("actions = %v", got)
	}
}

func TestReplyClassFor(t *testing.T) {
	cases := []struct{ kind, intent, want string }{
		{KindAutoReplyOOO, "", "out_of_office"},
		{KindAutoReplyTicket, "", "auto_reply"},
		{KindHumanReply, IntentAgreed, "positive"},
		{KindHumanReply, IntentScheduling, "positive"},
		{KindHumanReply, IntentNotInterested, "negative"},
		{KindHumanReply, IntentOptOut, "unsubscribe"},
		{KindHumanReply, IntentNotNow, "neutral"},
		{KindHumanReply, IntentUnclear, "neutral"},
		{KindNotification, "", "unknown"},
		{KindColdInbound, IntentAgreed, "unknown"},
	}
	for _, c := range cases {
		if got := ReplyClassFor(c.kind, c.intent); got != c.want {
			t.Errorf("ReplyClassFor(%s, %s) = %s, want %s", c.kind, c.intent, got, c.want)
		}
	}
}
