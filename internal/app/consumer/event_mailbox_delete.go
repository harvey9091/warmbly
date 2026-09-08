package jobs

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
)

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
