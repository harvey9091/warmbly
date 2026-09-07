package unibox

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func (s *uniboxService) Snooze(ctx context.Context, userID uuid.UUID, threadID string, until time.Time) (*models.UniboxSnooze, *errx.Error) {
	if threadID == "" {
		return nil, errx.New(errx.BadRequest, "thread_id is required")
	}
	now := time.Now()
	// Tiny lead-time grace so a click that takes a few hundred ms
	// over the wire doesn't blow up validation when the user picked
	// "in 1 minute" exactly.
	if until.Before(now.Add(5 * time.Second)) {
		return nil, errx.New(errx.BadRequest, "snoozed_until must be in the future")
	}
	if until.After(now.Add(SnoozeMaxHorizon)) {
		return nil, errx.New(errx.BadRequest, "snoozed_until is too far in the future (max 90 days)")
	}

	row, err := s.uniboxRepository.UpsertSnooze(ctx, userID, threadID, until.UTC())
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return row, nil
}

func (s *uniboxService) Unsnooze(ctx context.Context, userID uuid.UUID, threadID string) *errx.Error {
	if threadID == "" {
		return errx.New(errx.BadRequest, "thread_id is required")
	}
	if err := s.uniboxRepository.DeleteSnooze(ctx, userID, threadID); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}
	return nil
}

func (s *uniboxService) ListSnoozes(ctx context.Context, userID uuid.UUID) ([]models.UniboxSnooze, *errx.Error) {
	rows, err := s.uniboxRepository.ListSnoozes(ctx, userID)
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return rows, nil
}
