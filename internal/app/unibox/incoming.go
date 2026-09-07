package unibox

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func (s *uniboxService) Incoming(
	ctx context.Context,
	userID uuid.UUID,
	limit, cursor, from string,
) (*models.MailSearchResult, *errx.Error) {
	l, err := strconv.Atoi(limit)
	if err != nil {
		if limit != "" {
			return nil, errx.ErrUniboxLimit
		}
		l = DefaultLimit
	}

	if l < LimitMin || l > LimitMax {
		return nil, errx.ErrUniboxLimit
	}

	var resp *models.MailSearchResult

	if from != "" {
		resp, err = s.uniboxRepository.GetBySender(ctx, userID, from, l, cursor)
	} else {
		resp, err = s.uniboxRepository.GetIncoming(ctx, userID, l, cursor)
	}

	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	return resp, nil
}
