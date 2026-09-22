package inboxtag

import (
	"sort"
	"testing"
)

// Every label a decision can write must be in AllLabels, or it would be created
// one at a time as it first fires and could not be filtered on before that.
// This is the test that keeps labelsFor and AllLabels from drifting apart.
func TestAllLabelsCoversEverythingDecideCanWrite(t *testing.T) {
	all := map[string]bool{}
	for _, l := range AllLabels() {
		all[l] = true
	}

	// Every kind.
	for kind := range kindCriteria {
		if !all[slugOf(kind)] {
			t.Errorf("kind %q is not in AllLabels", kind)
		}
	}
	// Every intent.
	for intent := range intentCriteria {
		if !all[slugOf(intent)] {
			t.Errorf("intent %q is not in AllLabels", intent)
		}
	}
	if !all[LabelNeedsReview] {
		t.Error("needs-review is not in AllLabels")
	}

	// And the signals labelsFor actually surfaces, driven through the real
	// function rather than a second copy of the list.
	d := Decision{
		Kind:    KindHumanReply,
		Intent:  IntentAgreed,
		Signals: SignalIDs(),
	}
	for _, label := range labelsFor(d) {
		if !all[label] {
			t.Errorf("labelsFor emits %q, which AllLabels does not create", label)
		}
	}
}

func TestAllLabelsIsSortedAndUnique(t *testing.T) {
	labels := AllLabels()
	if !sort.StringsAreSorted(labels) {
		t.Error("AllLabels is not sorted, so the filter list order would drift between runs")
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if seen[l] {
			t.Errorf("duplicate label %q", l)
		}
		seen[l] = true
	}
	if len(labels) < 17 {
		t.Errorf("only %d labels; expected the full taxonomy", len(labels))
	}
}

// The first backfill over real mail put "needs-human-judgement" on bounces and
// platform notifications: the model answers the question honestly for any text,
// but the answer is meaningless on mail no person wrote, and a label that lands
// on everything is not a filter.
func TestSignalLabelsOnlyOnHumanReplies(t *testing.T) {
	signals := []string{SigNeedsHumanJudgement, SigAsksForCall, SigRequestsRemoval, SigLegalThreat}

	for _, kind := range []string{KindBounceHard, KindBounceSoft, KindNotification, KindAutoReplyOOO} {
		got := labelsFor(Decision{Kind: kind, Signals: signals})
		for _, l := range got {
			if l != slugOf(kind) {
				t.Errorf("kind %s got signal label %q; signals only label a human reply", kind, l)
			}
		}
	}

	human := labelsFor(Decision{Kind: KindHumanReply, Intent: IntentAgreed, Signals: signals})
	for _, want := range []string{"human-reply", "agreed", "asks-for-call", "needs-human-judgement"} {
		found := false
		for _, l := range human {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Errorf("a human reply lost the %q label: got %v", want, human)
		}
	}
}

// A kind the model is sure of survives an intent it is not sure of. The first
// backfill over real mail threw away four `human_reply` verdicts at 0.97
// confidence because the intent behind them sat at 0.6, which is the confident
// half being discarded along with the doubtful one.
func TestUnreadableIntentKeepsTheConfidentKind(t *testing.T) {
	answers := map[string]Answer{
		"kind":         {Type: QuestionChoice, Choice: KindHumanReply, Confidence: 0.97},
		"intent":       {Type: QuestionChoice, Choice: IntentWantsInfo, Confidence: 0.60},
		SigAsksForCall: {Type: QuestionNoul, Noul: 0.95},
	}
	d := Decide(answers, Facts{})

	if !d.NeedsReview {
		t.Error("an unreadable intent should still raise needs-review")
	}
	if d.ReviewReason != "intent" {
		t.Errorf("review reason = %q, want intent", d.ReviewReason)
	}
	if d.Intent != IntentWantsInfo {
		t.Errorf("intent %q not retained for review", d.Intent)
	}
	if d.Kind != KindHumanReply {
		t.Errorf("kind = %q; a 0.97 verdict should survive", d.Kind)
	}

	has := func(want string) bool {
		for _, l := range d.Labels {
			if l == want {
				return true
			}
		}
		return false
	}
	if !has("human-reply") {
		t.Errorf("lost the confident kind label: %v", d.Labels)
	}
	if !has(LabelNeedsReview) {
		t.Errorf("did not flag it for review: %v", d.Labels)
	}
	if has("wants-info") {
		t.Errorf("applied the untrusted intent as a label: %v", d.Labels)
	}
	if d.Relevance == 0 {
		t.Error("scored 0 despite a confident kind and a call request")
	}
}

// The dashboard keeps a hand-written explanation per label
// (web/src/lib/unibox/tagMeanings.ts) so a chip reading "going-cold" can say
// what it means on hover. That list cannot import this one, so this test pins
// the count: a label added here without an explanation there ships with no
// hover text, which is the state the feature started in.
func TestLabelCountMatchesTheDashboardMeanings(t *testing.T) {
	const documented = 28 // keep in step with EVERY_AUTOMATIC_LABEL in tagMeanings.test.ts
	if got := len(AllLabels()); got != documented {
		t.Fatalf("AllLabels has %d labels but the dashboard explains %d.\n"+
			"Add the new label to web/src/lib/unibox/tagMeanings.ts and to\n"+
			"EVERY_AUTOMATIC_LABEL in tagMeanings.test.ts, then update this count.",
			got, documented)
	}
}
