package socket

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func getTokenKey(id uuid.UUID) string {
	return "ws_verify:" + id.String()
}

// saveToken records the handshake nonce, and is best-effort on purpose.
//
// The nonce is already inside the signed token, and the realtime service
// verifies the token's signature and claims rather than looking this key up
// (realtime/lib/realtime/auth.ex). So the key is written for a revocation path
// that does not exist yet, and refusing to issue a token when it could not be
// written made an unreadable cache the single thing standing between every
// signed-in tab and live updates: a Redis provider over its request quota took
// realtime down for everyone and filed 2,815 issues doing it.
//
// It is still written, and still reported, so the day something does read it
// the key is there and an operator has been told when it was not.
func (s *socketService) saveToken(ctx context.Context, id uuid.UUID, nonce string, expiresAt time.Time) {
	if err := s.cache.SetEx(ctx, getTokenKey(id), nonce, time.Until(expiresAt)).Err(); err != nil {
		errs.CaptureException(err, errs.Tag("cache.key", "ws_verify"))
	}
}
