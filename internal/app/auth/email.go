package auth

import (
	"context"
	"strings"
	"time"
)

const authEmailSendTimeout = 10 * time.Second

// authEmailRetryDelay is the pause before a second attempt at a reset mail.
// Deliberately short: the handler holds the request open for 15s total, so the
// retry spends whatever is left of that and gives up when there is none. Long
// enough to ride out a refused connection, short enough that a rejection which
// is going to be permanent costs almost nothing.
const authEmailRetryDelay = 300 * time.Millisecond

// normalizeEmail is the one form an address is looked up and stored in:
// trimmed and lowercased. Addresses reach us from a typed form, a paste, a
// password manager and an IdP assertion, and none of those preserve the case
// the account was created with, so matching the raw string made
// "Vincent@Example.com" a different account from the one that exists. On the
// reset flow that was silent: an unknown address answers 200 by design, so the
// person was told the mail was sent and nothing ever arrived.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *authService) sendAuthEmail(ctx context.Context, to, subject, message string) error {
	ctx, cancel := context.WithTimeout(ctx, authEmailSendTimeout)
	defer cancel()

	return s.emailNotificationService.Send(ctx, []string{to}, nil, nil, subject, message)
}
