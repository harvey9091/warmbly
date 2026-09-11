package jobs

import (
	"context"
	"fmt"
	"slices"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

func (s *JobsService) HandleUpdateEmail(ctx context.Context, e *models.JobEventEmailUpdate) error {
	email, err := s.UniboxRepository.GetByID(ctx, e.UserID, e.ID)
	if err != nil {
		CaptureError(e.UserID, e.EmailID, fmt.Errorf("Email (%s): %w", e.ID.String(), err))
		return err
	}

	var updateData repository.UpdateUniboxEntry

	if !slices.Equal(email.Flags, e.Flags) {
		updateData.Flags = e.Flags
	}
	if email.UID != e.UID {
		updateData.UID = &e.UID
	}
	if email.Mailbox != e.Mailbox {
		updateData.Mailbox = &e.Mailbox
	}
	if email.ModSeq != e.ModSeq {
		updateData.ModSeq = &e.ModSeq
	}
	// The source folder's name, which is its identity. Empty on events from
	// workers predating the field, which keeps the stored value.
	if e.FolderPath != "" && email.FolderPath != e.FolderPath {
		updateData.FolderPath = &e.FolderPath
	}
	// A folder move follows the provider. Events from workers predating the
	// field carry "", which keeps the stored value. Delete/Archive in the
	// thread header only re-file the row here, so a later flag change on the
	// provider (still reporting inbox) must not pull the message back out.
	localMove := (email.Folder == models.FolderTrash || email.Folder == models.FolderArchive) &&
		e.Folder == models.FolderInbox
	followProvider := models.ValidFolder(e.Folder) && !localMove
	if followProvider && email.Folder != e.Folder {
		updateData.Folder = &e.Folder
	}

	if err := s.UniboxRepository.UpdateEntry(ctx, e.UserID, e.EmailID, e.ID, &updateData); err != nil {
		return err
	}

	email.Flags = e.Flags
	email.UID = e.UID
	email.Mailbox = e.Mailbox
	email.ModSeq = e.ModSeq
	if followProvider {
		email.Folder = e.Folder
	}
	s.publishEmailUpdated(ctx, e.UserID, email)
	return nil
}
