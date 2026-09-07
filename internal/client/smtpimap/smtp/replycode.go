package smtp

import (
	"errors"
	"net/textproto"
)

// replyCode is the three-digit status the server answered a command with, or
// 0 when the failure was not a reply at all (a dropped connection, a timeout,
// a TLS error). net/smtp surfaces every reply as a *textproto.Error.
func replyCode(err error) int {
	var proto *textproto.Error
	if errors.As(err, &proto) {
		return proto.Code
	}
	return 0
}

// permanentReply reports whether the server refused for good. 5xx means it
// will refuse the same message again, so retrying wastes the mailbox's daily
// budget and delays the campaign; 4xx explicitly invites a retry, and a
// failure with no reply code at all is the transport, which is also worth
// retrying.
//
// Reading the code is what lets a blocked sender or an over-quota mailbox be
// reported as what it is instead of as "the server may be offline", which is
// what every post-connection failure used to be called.
func permanentReply(err error) bool {
	code := replyCode(err)
	return code >= 500 && code < 600
}
