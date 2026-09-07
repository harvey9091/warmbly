package imap

import (
	"errors"

	"github.com/emersion/go-imap/v2"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

func (c *Client) handleError(err error) *errx.MailError {
	var imapErr *imap.Error
	if errors.As(err, &imapErr) {
		switch imapErr.Code {
		case imap.ResponseCodeAuthenticationFailed:
			if c.AuthType == models.AuthOAuth2 {
				return errx.ErrMailAuthenticationFailed
			} else {
				return errx.ErrMailInvalidCredentials
			}
		case imap.ResponseCodeAuthorizationFailed:
			return errx.ErrMailAuthorizationFailed
		default:
			return errx.ErrMailUnknownImapError(string(imapErr.Code))
		}
	}

	if err == nil {
		return nil
	}

	// Anything that is not a tagged IMAP response is the transport: a server
	// that dropped the session (net.ErrClosed once go-imap parks the client in
	// Logout), an EOF, a timeout. These used to map to nil, which turned a dead
	// connection into a "clean pass with no folders" — no log, no error record,
	// no new mail, forever. Retry-level, so the loop reconnects at the next
	// pass instead of deactivating the mailbox.
	return errx.ErrMailServerUnreachable
}
