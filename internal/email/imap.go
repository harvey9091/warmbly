package email

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/warmbly/warmbly/internal/client/netbind"
	"github.com/warmbly/warmbly/internal/models"
)

// VerifyImap probes a mailbox's IMAP credentials the way the sync client
// connects: the caller's security mode decides implicit TLS versus STARTTLS.
// security may be empty, in which case the port convention decides.
func VerifyImap(ctx context.Context, host string, port int, user, pass, security string) bool {
	// Brackets belong to the address, not to the host, and JoinHostPort is
	// what puts them back for an IPv6 literal.
	host = models.NormalizeMailHost(host)
	addr := models.MailDialAddress(host, port)

	resolved := models.ResolveIMAPSecurity(security, port)
	// The unencrypted mode only ever addresses this machine, checked here and
	// again against the peer below, so a mailbox that could never be dialled
	// safely fails at connect rather than on the first sync.
	if resolved == models.MailSecurityNone && !models.CleartextMailAllowed(host) {
		return false
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	if resolved == models.MailSecurityNone && !netbind.LoopbackPeer(conn) {
		return false
	}

	// Matches the sync client's TLS policy: MAIL_TLS_INSECURE is a dev-only
	// knob for the local self-signed sandbox, never set in production. Without
	// it, validation rejects mailboxes the worker would go on to sync fine.
	tlsConf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: netbind.InsecureTLS(),
	}

	var c *imapclient.Client

	if resolved == models.MailSecurityNone {
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		c = imapclient.New(conn, nil)
	} else if resolved == models.MailSecurityStartTLS {
		// The greeting arrives in cleartext and the upgrade happens in-band.
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		c, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsConf})
		if err != nil {
			return false
		}
	} else {
		tlsConn := tls.Client(conn, tlsConf)
		if err := tlsConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return false
		}
		c = imapclient.New(tlsConn, nil)
	}

	defer func() {
		_ = c.Logout().Wait()
		_ = c.Close()
	}()

	done := make(chan error, 1)
	go func() {
		done <- c.Login(user, pass).Wait()
	}()

	select {
	case err := <-done:
		return err == nil
	case <-ctx.Done():
		return false
	}
}
