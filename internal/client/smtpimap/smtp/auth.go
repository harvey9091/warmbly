package smtp

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/warmbly/warmbly/internal/models"
)

// ErrSMTPCleartextAuth is returned rather than sending credentials over an
// unencrypted link. Go's own PlainAuth refuses the same thing, but silently
// enough that operators read it as a credentials problem.
var ErrSMTPCleartextAuth = errors.New("smtp: refusing to send credentials over an unencrypted connection")

// ErrSMTPNoSupportedAuth means the server advertised AUTH but none of the
// mechanisms we implement.
var ErrSMTPNoSupportedAuth = errors.New("smtp: the server advertises no supported authentication mechanism")

// NegotiateAuth picks a mechanism from what the server actually advertised.
//
// Sending AUTH PLAIN blind, which is what net/smtp does when handed a
// PlainAuth, fails outright on a server that advertises only LOGIN, and
// Exchange Online relays and most appliance relays are in that group. The
// failure arrives as a rejected AUTH, which reads exactly like a wrong
// password, so the mailbox was deactivated and its owner told to check
// credentials that were correct.
//
// LOGIN is preferred over PLAIN because a server that speaks only one of the
// two speaks LOGIN. CRAM-MD5 is preferred over both where offered: it does not
// put the password on the wire at all.
//
// A server that advertises no AUTH at all gets a nil Auth and no error: it
// wants no authentication, which is the local development sink.
func NegotiateAuth(client *smtp.Client, username, password, host string) (smtp.Auth, error) {
	ok, ext := client.Extension("AUTH")
	if !ok {
		return nil, nil
	}
	mechs := strings.ToUpper(ext)
	switch {
	case strings.Contains(mechs, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(username, password), nil
	case strings.Contains(mechs, "LOGIN"):
		return NewLoginAuth(username, password, host), nil
	case strings.Contains(mechs, "PLAIN"):
		return smtp.PlainAuth("", username, password, host), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrSMTPNoSupportedAuth, ext)
}

// LoginAuth implements the non-standard but widely required AUTH LOGIN
// mechanism, which net/smtp does not ship. Exchange Online and many appliance
// relays advertise it exclusively.
type LoginAuth struct {
	username string
	password string
	host     string
}

// NewLoginAuth returns an AUTH LOGIN mechanism for the given credentials.
func NewLoginAuth(username, password, host string) smtp.Auth {
	return &LoginAuth{username: username, password: password, host: host}
}

func (a *LoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !IsLoopbackHost(server.Name) {
		return "", nil, ErrSMTPCleartextAuth
	}
	return "LOGIN", nil, nil
}

func (a *LoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	// Servers vary in how they word the prompts, so match on the decoded
	// challenge rather than expecting an exact string.
	challenge := strings.ToLower(string(fromServer))
	switch challenge {
	case "username:", "user name":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	if strings.Contains(challenge, "user") {
		return []byte(a.username), nil
	}
	if strings.Contains(challenge, "pass") {
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("smtp: unexpected LOGIN challenge %q", string(fromServer))
}

// IsLoopbackHost reports whether host is this machine, where credentials
// cannot reach a wire even without TLS. One definition, shared with the
// mailbox security modes, so the AUTH guard and the "none" mode can never
// disagree about what counts as local.
func IsLoopbackHost(host string) bool {
	return models.LoopbackMailHost(host)
}
