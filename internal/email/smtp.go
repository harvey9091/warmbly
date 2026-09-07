package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"

	"github.com/warmbly/warmbly/internal/client/netbind"
	wsmtp "github.com/warmbly/warmbly/internal/client/smtpimap/smtp"
	"github.com/warmbly/warmbly/internal/models"
)

// VerifySMTP probes a mailbox's SMTP credentials the same way the send path
// connects: the caller's security mode decides implicit TLS versus STARTTLS,
// and any port is accepted. security may be empty, in which case the port
// convention decides.
func VerifySMTP(ctx context.Context, host string, port int, user, pass, security string) bool {
	addr := fmt.Sprintf("%s:%d", host, port)

	// Matches the send client's TLS policy: MAIL_TLS_INSECURE is a dev-only
	// knob for the local self-signed sandbox, never set in production.
	tlsConf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: netbind.InsecureTLS(), //nolint:gosec // MAIL_TLS_INSECURE, local dev only
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	var err error

	// netbind dialers so validation probes leave from WORKER_BIND_IP exactly
	// like the sends they are vouching for.
	implicitTLS := models.ResolveSMTPSecurity(security, port) == models.MailSecurityTLS
	if implicitTLS {
		conn, err = netbind.TLSDialer(nil, tlsConf).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = netbind.Dialer(nil).DialContext(ctx, "tcp", addr)
	}
	// A bad host is ordinary user input, not an exceptional case: dial failed
	// means conn is nil, and closing it would panic this goroutine and take
	// the whole worker down with it.
	if err != nil || conn == nil {
		return false
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return false
	}
	defer c.Close()

	if !implicitTLS {
		// TLS stays mandatory, with the same dev-only escape hatch the send
		// path uses for the local no-STARTTLS sink.
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsConf); err != nil {
				return false
			}
		} else if !netbind.InsecureTLS() {
			return false
		}
	}

	// Negotiated from what the server advertised, like the send path: a
	// server that offers only LOGIN refuses a blind AUTH PLAIN, and probing
	// with PLAIN alone rejected mailboxes whose credentials were correct.
	auth, aerr := wsmtp.NegotiateAuth(c, user, pass, host)
	if aerr != nil {
		return false
	}
	if auth == nil {
		// No AUTH offered at all: nothing to verify, and the send path will
		// not authenticate either.
		return true
	}

	done := make(chan error, 1)
	go func() { done <- c.Auth(auth) }()

	select {
	case err := <-done:
		return err == nil
	case <-ctx.Done():
		return false
	}
}
