package jobs

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
)

// HandleMailboxRename follows a folder the server renamed.
//
// A folder is keyed by name, so a rename would otherwise read as one folder
// disappearing and another appearing, which orphans every message filed under
// the old name and re-imports the folder's history under the new one. An IMAP
// RENAME keeps UIDVALIDITY and every UID, so there is nothing to re-import:
// the row and the mail both just move, and they move together or not at all.
//
// A rename that finds nothing to move is not an error. The destination name
// already existing means the listing showed both folders at once, which is
// not a rename, and the ordinary insert and delete paths handle it.
func (s *JobsService) HandleMailboxRename(ctx context.Context, e *models.JobEventMailboxRename) error {
	if e.From == "" || e.To == "" || e.From == e.To {
		return nil
	}
	renamed, err := s.MailboxRepository.RenameMailbox(ctx, e.UserID, e.EmailID, e.From, e.To)
	if err != nil {
		CaptureError(e.UserID, e.EmailID, err)
		return err
	}
	if !renamed {
		log.Info().
			Str("email_id", e.EmailID.String()).
			Str("from", e.From).
			Str("to", e.To).
			Msg("mailbox rename had nothing to move; the destination name is already taken or the source is gone")
	}
	return nil
}
