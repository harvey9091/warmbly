package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/pkg/warmlint"
	"github.com/warmbly/warmbly/internal/repository"
)

// contentWarningWindow is how long one warning covers a step, so a campaign
// with 500 recipients writes one feed entry rather than 500.
const contentWarningWindow = 24 * time.Hour

// warnOnWeakContent scores the copy this send will actually deliver — after
// merge fields, spintax, A/B and AI blocks resolve, which the editor's
// template-time score cannot see. Advisory: it never blocks or delays a send.
func (s *tasksService) warnOnWeakContent(ctx context.Context, orgID, campaignID, sequenceID uuid.UUID, step int, subject, bodyHTML, bodyPlain string, attachments int) {
	if s.advanced == nil || s.campaignLogRepo == nil {
		return
	}
	// A clean 100 clears every floor, so the common case never reads settings.
	res := warmlint.ScoreWithAttachments(subject, bodyHTML, bodyPlain, attachments)
	if res.Score >= 100 {
		return
	}

	// Campaign-effective, not org-only: a campaign that turned the check off or
	// moved its floor must be honored here as it is at preflight.
	settings, xerr := s.advanced.EffectiveSettings(ctx, orgID, campaignID)
	if xerr != nil || settings == nil || !settings.Preflight.Enabled || !settings.Preflight.CheckContentScore {
		return
	}
	floor := settings.Preflight.MinContentScore
	// Out of range means a row written before the floor was clamped.
	if floor <= 0 || floor > 100 {
		floor = 60
	}
	if res.Score >= floor {
		return
	}

	seq := sequenceID.String()
	detail := ""
	if lead := warmlint.LeadIssue(res); lead != "" {
		detail = " " + lead
	}

	codes := make([]string, 0, len(res.Issues))
	for _, issue := range res.Issues {
		codes = append(codes, issue.Code)
	}

	if _, err := s.campaignLogRepo.CreateLogOnce(ctx, &repository.CampaignLogEntry{
		CampaignID: campaignID,
		EventType:  "content_warning",
		Message: fmt.Sprintf("Step %d's copy scores %d/100 for spam signals as sent (floor %d).%s",
			step, res.Score, floor, detail),
		Metadata: map[string]interface{}{
			// "warn" is the dashboard's amber tier; anything else reads as info.
			"level":       "warn",
			"sequence_id": seq,
			"score":       res.Score,
			"floor":       floor,
			"issues":      codes,
		},
	}, "sequence_id", seq, time.Now().Add(-contentWarningWindow)); err != nil {
		log.Warn().Err(err).Str("campaign_id", campaignID.String()).Msg("could not record the content warning")
	}
}
