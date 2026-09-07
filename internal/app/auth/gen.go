package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/pkg/argon2"
)

func (s *authService) GenerateLoginSession(ctx context.Context, userID uuid.UUID, code string) (string, *errx.Error) {
	sessID := uuid.New()

	issuedAt := time.Now()
	expiresAt := issuedAt.Add(SessionTTL)

	codeHash, err := argon2.Hash(code)
	if err != nil {
		errs.CaptureException(err)
		return "", errx.InternalError()
	}

	loginSession := &models.LoginSession{
		CodeHash: codeHash,
		Tries:    0,
	}

	if err := s.saveLoginSession(ctx, sessID, loginSession, expiresAt); err != nil {
		return "", err
	}

	sessionToken, err := s.tokenService.GenerateToken(userID, sessID, "", "", issuedAt, expiresAt)
	if err != nil {
		errs.CaptureException(err)
		return "", errx.InternalError()
	}

	return sessionToken, nil
}
