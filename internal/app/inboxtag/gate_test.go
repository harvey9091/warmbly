package inboxtag

import (
	"encoding/json"
	"testing"

	"github.com/warmbly/warmbly/internal/repository"
)

func storedRow(kind, intent string, answers map[string]Answer) *repository.InboxTagResult {
	raw, _ := json.Marshal(answers)
	return &repository.InboxTagResult{
		Kind: kind, KindConfidence: 0.95, Intent: intent, IntentConfidence: 0.9, Answers: raw,
	}
}

func TestDraftDecision(t *testing.T) {
	cases := []struct {
		name string
		row  *repository.InboxTagResult
		want bool
	}{
		{"untrusted verdict drafts", &repository.InboxTagResult{Kind: KindHumanReply, KindConfidence: 0.4}, true},
		{"bounce", storedRow(KindBounceHard, "", nil), false},
		{"closed intent", storedRow(KindHumanReply, IntentNotInterested, nil), false},
		{"open intent", storedRow(KindHumanReply, IntentWantsInfo, nil), true},
		{"legal threat", storedRow(KindHumanReply, IntentWantsInfo, map[string]Answer{SigLegalThreat: {Noul: 0.7}}), false},
		{"needs a person", storedRow(KindHumanReply, IntentWantsInfo, map[string]Answer{SigNeedsHumanJudgement: {Noul: 0.9}}), false},
		{"one-line ack", storedRow(KindHumanReply, IntentQuestionAnswered, map[string]Answer{ScoreReplyEffort: {Score: 0}}), false},
		{"real write-up", storedRow(KindHumanReply, IntentQuestionAnswered, map[string]Answer{ScoreReplyEffort: {Score: 2}}), true},
	}
	for _, c := range cases {
		got, why := DraftDecision(c.row)
		if got != c.want {
			t.Errorf("%s: draft=%v (%s), want %v", c.name, got, why, c.want)
		}
	}
}
