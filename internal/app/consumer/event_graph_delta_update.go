package jobs

import (
	"context"

	"github.com/warmbly/warmbly/internal/models"
)

// HandleGraphDeltaUpdate persists the opaque per-folder Microsoft Graph delta
// cursor relayed by the worker, so the mailbox resumes from its last position on
// the next (re)assignment instead of re-priming.
func (s *JobsService) HandleGraphDeltaUpdate(ctx context.Context, e *models.JobEventGraphDeltaUpdate) error {
	if s.EmailGraphDeltaRepository == nil {
		return nil
	}
	if err := s.EmailGraphDeltaRepository.Put(ctx, e.UserID, e.EmailID, e.Folder, e.DeltaLink); err != nil {
		if s.dropForDeletedMailbox(ctx, e.UserID, e.EmailID, err) {
			return nil
		}
		CaptureError(e.UserID, e.EmailID, err)
		return err
	}
	return nil
}
