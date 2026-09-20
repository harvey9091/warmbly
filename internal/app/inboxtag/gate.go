package inboxtag

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/repository"
)

// DraftGate answers whether a reply is worth a paid draft, from the stored
// verdict: no credits on a closed reply, a one-line ack or a legal threat.
type DraftGate struct {
	repo repository.InboxTagRepository
}

func NewDraftGate(repo repository.InboxTagRepository) *DraftGate {
	return &DraftGate{repo: repo}
}

// ShouldDraft reports whether to draft, and the reason when not. A message
// with no stored verdict is drafted: the gate only ever declines on evidence.
func (g *DraftGate) ShouldDraft(ctx context.Context, orgID uuid.UUID, messageID string) (bool, string) {
	if g == nil || g.repo == nil || messageID == "" {
		return true, ""
	}
	stored, err := g.repo.GetByMessageID(ctx, orgID, messageID)
	if err != nil || stored == nil {
		return true, ""
	}
	return DraftDecision(stored)
}

// DraftDecision is the pure half, so the rules can be tested against a row.
func DraftDecision(r *repository.InboxTagResult) (bool, string) {
	if r.KindConfidence < ConfFloor || r.NeedsReview {
		return true, ""
	}
	if r.Kind != KindHumanReply {
		return false, "not a human reply: " + r.Kind
	}
	if r.IntentConfidence >= ConfFloor && IsClosedIntent(r.Intent) {
		return false, "the reply closed the conversation: " + r.Intent
	}

	var answers map[string]Answer
	if len(r.Answers) > 0 {
		_ = json.Unmarshal(r.Answers, &answers)
	}
	if a, ok := answers[SigLegalThreat]; ok && a.Noul >= Yes {
		return false, "legal threat: a person reads this first"
	}
	if a, ok := answers[SigNeedsHumanJudgement]; ok && a.Noul >= Strong {
		return false, "needs human judgement"
	}
	if a, ok := answers[ScoreReplyEffort]; ok && a.Normalized(ScoreLevels(ScoreReplyEffort)) == 0 {
		return false, "a one-line acknowledgement is not worth a draft"
	}
	return true, ""
}
