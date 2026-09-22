package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/infrastructure/pubsub"
	"github.com/warmbly/warmbly/internal/models"
)

// publishFolderPurged tells open dashboards that rows left a mailbox in bulk.
// One org-scoped EMAIL_DELETED with no message id: the client drops every
// inbox list on that event whatever it names, and one event is what a purge
// of a whole folder deserves rather than one per row.
func (s *JobsService) publishFolderPurged(ctx context.Context, userID, emailID uuid.UUID) {
	if s.StreamingPublisher == nil {
		return
	}
	var orgID string
	if account, err := s.EmailRepository.GetByID(ctx, emailID); err == nil && account != nil && account.OrganizationID != nil {
		orgID = account.OrganizationID.String()
	}
	s.StreamingPublisher.PublishEmailDeleted(ctx, &pubsub.EmailInboxEvent{
		BaseEvent:      pubsub.BaseEvent{UserID: userID.String()},
		OrgID:          orgID,
		EmailAccountID: emailID.String(),
	})
}

// HandleMailboxDelete retires a folder the last listing no longer had.
//
// A folder is identified by its name. UIDValidity is the fallback for an
// event from a worker deployed before that was true, which names no folder at
// all; it only deletes when the number matches exactly one folder, because it
// no longer identifies one on a server that stamps it from a creation time.
// An ambiguous one is left alone rather than retried: nothing about it will
// change, and the next pass from an updated worker retires the folder by
// name.
func (s *JobsService) HandleMailboxDelete(ctx context.Context, e *models.JobEventMailboxDelete) error {
	if e.Mailbox != "" {
		if err := s.MailboxRepository.DeleteMailbox(ctx, e.UserID, e.EmailID, e.Mailbox); err != nil {
			CaptureError(e.UserID, e.EmailID, err)
			return err
		}
		// A folder the owner excluded from sync is retired with the mail
		// already stored from it; the mail itself stays at the provider.
		if e.Skipped && s.UniboxRepository != nil {
			n, err := s.UniboxRepository.DeleteByFolderPaths(ctx, e.EmailID, []string{e.Mailbox})
			if err != nil {
				CaptureError(e.UserID, e.EmailID, err)
				return err
			}
			log.Info().
				Str("email_id", e.EmailID.String()).
				Str("folder", e.Mailbox).
				Int64("messages", n).
				Msg("folder excluded from sync: stored mail dropped")
			if n > 0 {
				s.publishFolderPurged(ctx, e.UserID, e.EmailID)
			}
		}
		return nil
	}

	removed, err := s.MailboxRepository.DeleteMailboxByUIDValidity(ctx, e.UserID, e.EmailID, e.UIDValidity)
	if err != nil {
		CaptureError(e.UserID, e.EmailID, err)
		return err
	}
	if removed == 0 {
		log.Info().
			Str("email_id", e.EmailID.String()).
			Uint32("uid_validity", e.UIDValidity).
			Msg("legacy mailbox delete skipped: that UIDVALIDITY does not name exactly one folder")
	}
	return nil
}
