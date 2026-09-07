package socket

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/pkg/crypt"
)

func (s *socketService) GenerateWebsocketToken(ctx context.Context, userID uuid.UUID) (string, *errx.Error) {
	issuedAt := time.Now()
	expiresAt := issuedAt.Add(SocketTTL)
	id := uuid.New()
	nonce, err := crypt.Nonce()
	if err != nil {
		errs.CaptureException(err)
		return "", errx.InternalError()
	}

	wsToken, err := s.tokenService.GenerateToken(userID, id, "", nonce, issuedAt, expiresAt)
	if err != nil {
		errs.CaptureException(err)
		return "", errx.InternalError()
	}

	if err := s.saveToken(ctx, id, nonce, expiresAt); err != nil {
		return "", err
	}

	return wsToken, nil
}
