package jobs

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/infrastructure/pubsub"
	"github.com/warmbly/warmbly/internal/models"
)

// HandleRemoveEmail processes a message removal observed during mailbox sync.
//
// Tampering protection: if the removed message was a warmup email (tracked in
// warmup_received), the recipient deleted pool warmup mail. That is recorded
// as a strike and the health bands decide: one deletion warns, more pauses or
// blocks. The owner can appeal a block.
//
// It also drops the local unibox entry for the removed message (best-effort).
func (s *JobsService) HandleRemoveEmail(ctx context.Context, e *models.JobEventRemoveEmail) error {
	if s.WarmupRepo != nil {
		if rec, _ := s.WarmupRepo.GetWarmupReceived(ctx, e.EmailID, e.ID); rec != nil {
			switch {
			case s.consumeSelfMove(ctx, e.EmailID, rec.MessageID):
				// Our own engagement foldered this message. Providers that
				// report a move as a removal (Graph) would otherwise ban the
				// recipient for the action we asked it to perform.
				log.Debug().
					Str("email_id", e.EmailID.String()).
					Str("message_id", rec.MessageID).
					Msg("Warmup message left its folder because we moved it; not tampering")
			case s.WarmupService != nil:
				health, _ := s.WarmupService.RecordTampering(ctx, e.EmailID, rec.MessageID, "deletion")
				s.markRiskBandFromWarmupHealth(ctx, e.EmailID, health)
			}
		}
	}

	if s.UniboxRepository != nil {
		_ = s.UniboxRepository.Delete(ctx, e.UserID, e.ID)
	}

	// Tell open dashboards the row is gone (org-scoped so every teammate's
	// unibox drops it live, not just the mailbox owner's).
	if s.StreamingPublisher != nil {
		var orgID string
		if account, err := s.EmailRepository.GetByID(ctx, e.EmailID); err == nil && account != nil && account.OrganizationID != nil {
			orgID = account.OrganizationID.String()
		}
		s.StreamingPublisher.PublishEmailDeleted(ctx, &pubsub.EmailInboxEvent{
			BaseEvent:      pubsub.BaseEvent{UserID: e.UserID.String()},
			OrgID:          orgID,
			EmailAccountID: e.EmailID.String(),
			MessageID:      e.ID.String(),
		})
	}
	return nil
}
