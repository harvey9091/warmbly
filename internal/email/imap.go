package email

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/warmbly/warmbly/internal/client/netbind"
	"github.com/warmbly/warmbly/internal/models"
)

// VerifyImap probes a mailbox's IMAP credentials the way the sync client
// connects: the caller's security mode decides implicit TLS versus STARTTLS.
// security may be empty, in which case the port convention decides. The
// result says why a probe failed, so a refused password and an unreachable
// host are not reported as the same thing.
func VerifyImap(ctx context.Context, host string, port int, user, pass, security string) ProbeResult {
	// Brackets belong to the address, not to the host, and JoinHostPort is
	// what puts them back for an IPv6 literal.
	host = models.NormalizeMailHost(host)
	addr := models.MailDialAddress(host, port)

	resolved := models.ResolveIMAPSecurity(security, port)
	// The unencrypted mode only ever addresses this machine, checked here and
	// again against the peer below, so a mailbox that could never be dialled
	// safely fails at connect rather than on the first sync.
	if resolved == models.MailSecurityNone && !models.CleartextMailAllowed(host) {
		return probeFailText(models.MailProbeCleartext, "unencrypted IMAP is only allowed to a loopback host on a self-hosted instance")
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return probeFail(ctx, models.MailProbeUnreachable, err)
	}
	defer conn.Close()
	if resolved == models.MailSecurityNone && !netbind.LoopbackPeer(conn) {
		return probeFailText(models.MailProbeCleartext, "the host did not resolve to this machine")
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
			return probeFail(ctx, models.MailProbeProtocol, err)
		}
		c = imapclient.New(conn, nil)
	} else if resolved == models.MailSecurityStartTLS {
		// The greeting arrives in cleartext and the upgrade happens in-band.
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return probeFail(ctx, models.MailProbeProtocol, err)
		}
		c, err = imapclient.NewStartTLS(conn, &imapclient.Options{TLSConfig: tlsConf})
		if err != nil {
			return probeFail(ctx, models.MailProbeTLS, err)
		}
	} else {
		tlsConn := tls.Client(conn, tlsConf)
		if err := tlsConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return probeFail(ctx, models.MailProbeProtocol, err)
		}
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return probeFail(ctx, models.MailProbeTLS, err)
		}
		c = imapclient.New(tlsConn, nil)
	}

	defer c.Close()

	done := make(chan error, 1)
	go func() {
		done <- c.Login(user, pass).Wait()
	}()

	select {
	case err := <-done:
		_ = c.Logout().Wait()
		if err == nil {
			return probeOK()
		}
		return probeFail(ctx, imapLoginReason(err), err)
	case <-ctx.Done():
		// No logout: the server has not answered the login, and waiting for
		// it to answer that would run out the socket deadline instead.
		return probeFail(ctx, models.MailProbeTimeout, ctx.Err())
	}
}

// imapLoginReason reads a LOGIN refusal. A status response is the server's
// answer to the credentials; anything else is the conversation breaking.
func imapLoginReason(err error) string {
	var ierr *imap.Error
	if !errors.As(err, &ierr) {
		return models.MailProbeProtocol
	}
	switch ierr.Code {
	case imap.ResponseCodeUnavailable, imap.ResponseCodeLimit, imap.ResponseCodeInUse:
		return models.MailProbeTemporary
	}
	if ierr.Type == imap.StatusResponseTypeBye {
		return models.MailProbeTemporary
	}
	return models.MailProbeAuthRefused
}
