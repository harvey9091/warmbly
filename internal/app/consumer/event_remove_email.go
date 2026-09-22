package jobs

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/infrastructure/pubsub"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// HandleRemoveEmail processes a message removal observed during mailbox sync.
//
// Tampering protection: if the removed message was a warmup email (tracked in
// warmup_received) and it went soon after it arrived, the recipient deleted
// pool warmup mail before its engagement was earned. That is recorded as a
// strike and the health bands decide: one deletion warns, more pauses or
// blocks. The owner can appeal a block. A removal later than that is
// housekeeping (see warmupDeletionCounts) and is not held against anyone.
//
// It also drops the local unibox entry for the removed message (best-effort).
func (s *JobsService) HandleRemoveEmail(ctx context.Context, e *models.JobEventRemoveEmail) error {
	// A message the sync found in a folder the owner excluded is filed, not
	// deleted: it is still in the mailbox, so nothing is held against anyone.
	if s.WarmupRepo != nil && e.SkippedFolder == "" {
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
			case !warmupDeletionCounts(rec, time.Now()):
				log.Debug().
					Str("email_id", e.EmailID.String()).
					Str("message_id", rec.MessageID).
					Msg("Warmup message removed after its engagement window; housekeeping, not tampering")
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

// warmupDeletionCounts decides whether a deletion of a received warmup message
// is tampering. It is when the message is still fresh: the engagement legs
// run inside the first hours, and removing the mail before then costs the
// pool the signal it was sent for. Past config.WarmupDeletionStrikeHours the
// platform's own retention is going to delete it anyway, so an owner tidying
// the folder, Gmail purging its Trash or a server retention rule is doing the
// platform's job early, not harm. A message the retention sweep has already
// retired is the platform's own deletion whenever it is observed.
func warmupDeletionCounts(rec *repository.WarmupReceived, now time.Time) bool {
	if rec == nil || rec.RetiredAt != nil {
		return false
	}
	return now.Sub(rec.CreatedAt) < time.Duration(config.WarmupDeletionStrikeHours)*time.Hour
}
