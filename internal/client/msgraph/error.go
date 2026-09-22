package msgraph

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/errx"
)

// graphErrorEnvelope is Graph's standard error shape.
type graphErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// HandleError maps a non-2xx Graph response to a MailError. It classifies by
// HTTP status so the send/sync retry loop can tell transient (retryable) from
// critical (needs re-auth / stop) failures:
//   - 401 -> authentication failed (token expired/revoked, re-consent)
//   - 403 -> authorization failed (missing scope / mailbox disabled)
//   - 404 -> resource not found (a folder the tenant does not have, a message
//     already gone); its own code so a caller can skip what is absent without
//     also skipping on a 503
//   - 429 -> sending too fast (throttled; Retry-After honored by the caller loop)
//   - 5xx / other -> server unreachable (retry)
func HandleError(resp *http.Response) *errx.MailError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var env graphErrorEnvelope
	_ = json.Unmarshal(body, &env)

	logError := func() {
		log.Debug().
			Int("status", resp.StatusCode).
			Str("code", env.Error.Code).
			Str("message", env.Error.Message).
			Msg("Graph API error")
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errx.ErrMailAuthenticationFailed
	case http.StatusForbidden:
		return errx.ErrMailAuthorizationFailed
	case http.StatusNotFound:
		logError()
		return errx.ErrMailResourceNotFound
	case http.StatusTooManyRequests:
		mailErr := errx.MError(
			errx.ErrMailSendingTooFast.Type,
			errx.ErrMailSendingTooFast.Code,
			errx.ErrMailSendingTooFast.Message,
			errx.ErrMailSendingTooFast.ResolveMethod,
		)
		mailErr.RetryAfter = retryAfter(resp.Header.Get("Retry-After"), time.Now())
		return mailErr
	default:
		logError()
		return errx.ErrMailServerUnreachable
	}
}

func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
