package user

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

func (s *userService) GetUser(ctx context.Context, userID uuid.UUID) (*models.User, *errx.Error) {
	u, err := s.getUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	u, xerr := s.userRepository.GetUser(ctx, userID)
	if xerr != nil {
		// The repository already says "no such account" for a missing row.
		// Flattening that into an internal fault answered a deleted or
		// mistyped user with "Something went wrong", which reads as a broken
		// server rather than as the account not being there.
		var bizErr *errx.Error
		if errors.As(xerr, &bizErr) {
			return nil, bizErr
		}
		return nil, errx.InternalError()
	}

	if err := s.SaveUser(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}
