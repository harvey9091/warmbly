package unibox

import (
	"context"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func (s *uniboxService) MarkSeen(ctx context.Context, userID, emailID uuid.UUID, seen bool) *errx.Error {
	if err := s.uniboxRepository.MarkSeen(ctx, userID, emailID, seen); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	return nil
}

func (s *uniboxService) MarkSeenBulk(ctx context.Context, orgID uuid.UUID, data *models.MarkSeen) (*models.MarkSeen, *errx.Error) {
	if len(data.EmailIDs) > 500 {
		return nil, errx.ErrSeenMax
	}

	// A folder sweep and an id list are different requests; refuse the
	// ambiguous combination instead of guessing which one was meant.
	if data.Folder != "" {
		if len(data.EmailIDs) > 0 {
			return nil, errx.ErrSeenFolderAndIDs
		}
		if !models.ValidFolder(data.Folder) {
			return nil, errx.ErrUniboxFolder
		}
		if err := s.uniboxRepository.MarkSeenByFolder(ctx, orgID, data.Folder, data.Seen); err != nil {
			errs.CaptureException(err)
			return nil, errx.InternalError()
		}
		return data, nil
	}

	if err := s.uniboxRepository.MarkSeenBulk(ctx, orgID, data.EmailIDs, data.Seen); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	return data, nil
}

// MoveFolderBulk backs Archive, Delete and Move to inbox in the thread header.
// Store-side only: the provider copy stays where it is, and provider_folder is
// left alone so the sync can still tell a real provider move from a flag scan.
func (s *uniboxService) MoveFolderBulk(ctx context.Context, orgID uuid.UUID, data *models.MoveFolder) (*models.MoveFolder, *errx.Error) {
	if len(data.EmailIDs) > 500 {
		return nil, errx.ErrSeenMax
	}
	// Only the three a user can file into. sent/drafts/spam are verdicts the
	// provider reaches, and accepting them here would let a caller forge one.
	if !models.FilableFolder(data.Folder) {
		return nil, errx.ErrUniboxFilableFolder
	}
	if err := s.uniboxRepository.MoveToFolderBulk(ctx, orgID, data.EmailIDs, data.Folder); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return data, nil
}
