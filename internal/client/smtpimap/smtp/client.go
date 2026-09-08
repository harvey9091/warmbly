package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/warmbly/warmbly/internal/client/netbind"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/mailhdr"
	"golang.org/x/oauth2"
)

// sendTimeout bounds one whole SMTP conversation: dial, greeting, AUTH, the
// recipients, the message and the server's verdict. It is generous because a
// large message to a slow relay is legitimate, and it exists so a peer that
// stops answering cannot hold a send goroutine forever.
const sendTimeout = 90 * time.Second

// ehloName is the domain to announce in EHLO, taken from the sender's own
// address. Empty leaves net/smtp's default in place.
func ehloName(address string) string {
	if at := strings.LastIndex(address, "@"); at >= 0 && at+1 < len(address) {
		return address[at+1:]
	}
	return ""
}

type Client struct {
	FirstName string
	LastName  string
	Email     string

	AuthType    models.AuthType
	Credentials *models.Service
	Oauth2      *models.Oauth2Service

	// plaintext skips the STARTTLS requirement. Only the tests set it, to
	// talk to an in-process server; no product path reaches it, because SMTP
	// AUTH in the clear would put the mailbox password on the wire.
	plaintext bool

	// BindIP optionally pins outbound TCP to a specific local source address.
	// When nil, WORKER_BIND_IP is consulted; when still unset, the OS default
	// route is used.
	BindIP *net.TCPAddr
}

// Attachment is a fully-resolved file to encode into an outbound message. Data
// is the raw bytes; MimeType drives the Content-Type.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// Send submits the message and returns the exact RFC 5322 bytes it put on the
// wire, so the caller can file the same copy in the mailbox's Sent folder.
// The bytes are returned even on failure: they are useful in a log, and a
// caller that ignores them is unaffected.
func (c *Client) Send(
	ctx context.Context,
	fromName string,
	to, cc, bcc []string,
	messageID,
	subject, bodyPlain, bodyHTML,
	inReplyTo string,
	attachments []Attachment,
	customHeaders ...map[string]string,
) ([]byte, *errx.MailError) {
	// A per-send name (the mailbox as renamed in the dashboard) wins over the
	// one cached at load time.
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = strings.TrimSpace(c.FirstName + " " + c.LastName)
	}
	from := mail.Address{Address: c.Email, Name: fromName}

	// ----- Headers -----
	headers := map[string]string{
		"From": from.String(),
		"To":   mailhdr.AddressList(to),
		// Headers are ASCII on the wire: a raw accent or emoji in the subject
		// arrives as mojibake, so RFC 2047-encode it (a no-op for plain ASCII).
		"Subject":      mailhdr.Subject(subject),
		"Date":         time.Now().Format(time.RFC1123Z),
		"MIME-Version": "1.0",
	}
	if messageID != "" {
		// Write the caller's Message-ID so the id we record matches what was
		// actually sent; without it the receiving server assigns its own and
		// reply/bounce correlation and threading break.
		headers["Message-ID"] = "<" + strings.Trim(messageID, "<>") + ">"
	}
	if len(cc) > 0 {
		headers["Cc"] = mailhdr.AddressList(cc)
	}
	if inReplyTo != "" {
		// The parent Message-ID may arrive already wrapped in <...>; trim before
		// re-wrapping so we don't emit <<id>>, which won't match the original
		// Message-ID header and breaks Gmail/Outlook threading.
		mid := "<" + strings.Trim(inReplyTo, "<>") + ">"
		headers["In-Reply-To"] = mid
		headers["References"] = mid
	}

	// Add custom headers (e.g., X-Warmbly-Token for warmup)
	if len(customHeaders) > 0 {
		for k, v := range customHeaders[0] {
			headers[k] = v
		}
	}

	var msg bytes.Buffer
	if len(attachments) > 0 {
		c.writeMixedBody(&msg, headers, bodyPlain, bodyHTML, attachments)
	} else {
		c.writeAlternativeBody(&msg, headers, bodyPlain, bodyHTML)
	}

	// Envelope recipients are bare addresses: a display name belongs in the
	// header, and "Ana <a@b.com>" in RCPT TO is rejected by the server.
	recipients := mailhdr.BareList(to)
	recipients = append(recipients, mailhdr.BareList(cc)...)
	recipients = append(recipients, mailhdr.BareList(bcc)...)

	raw := msg.Bytes()
	return raw, c.sendRaw(ctx, from.Address, recipients, raw)
}

// writeAlternativeBody writes a multipart/alternative message (text/plain +
// optional text/html) including the top-level headers.
func (c *Client) writeAlternativeBody(msg *bytes.Buffer, headers map[string]string, bodyPlain, bodyHTML string) {
	writer := multipart.NewWriter(msg)
	headers["Content-Type"] = fmt.Sprintf("multipart/alternative; boundary=%s", writer.Boundary())

	for k, v := range headers {
		fmt.Fprintf(msg, "%s: %s\r\n", k, v)
	}
	fmt.Fprint(msg, "\r\n")

	writeTextParts(writer, bodyPlain, bodyHTML)
	writer.Close()
}

// writeMixedBody writes a multipart/mixed message: a multipart/alternative
// sub-tree for the text bodies, then one application/* part per attachment with
// a Content-Disposition: attachment header.
func (c *Client) writeMixedBody(msg *bytes.Buffer, headers map[string]string, bodyPlain, bodyHTML string, attachments []Attachment) {
	mixed := multipart.NewWriter(msg)
	headers["Content-Type"] = fmt.Sprintf("multipart/mixed; boundary=%s", mixed.Boundary())

	for k, v := range headers {
		fmt.Fprintf(msg, "%s: %s\r\n", k, v)
	}
	fmt.Fprint(msg, "\r\n")

	// multipart/alternative sub-tree for the text bodies.
	var altBuf bytes.Buffer
	alt := multipart.NewWriter(&altBuf)
	writeTextParts(alt, bodyPlain, bodyHTML)
	alt.Close()

	altPart, _ := mixed.CreatePart(textproto.MIMEHeader{
		"Content-Type": {fmt.Sprintf("multipart/alternative; boundary=%s", alt.Boundary())},
	})
	altPart.Write(altBuf.Bytes())

	// One attachment part per file.
	for _, a := range attachments {
		mimeType := a.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		fn := mime.QEncoding.Encode("utf-8", a.Filename)
		part, _ := mixed.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {fmt.Sprintf("%s; name=%q", mimeType, fn)},
			"Content-Transfer-Encoding": {"base64"},
			"Content-Disposition":       {fmt.Sprintf("attachment; filename=%q", fn)},
		})
		writeBase64Wrapped(part, a.Data)
	}

	mixed.Close()
}

// writeTextParts writes the text/plain and optional text/html quoted-printable
// parts into the given multipart writer.
func writeTextParts(writer *multipart.Writer, bodyPlain, bodyHTML string) {
	if bodyPlain != "" {
		part, _ := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"text/plain; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		qp := quotedprintable.NewWriter(part)
		qp.Write([]byte(bodyPlain))
		qp.Close()
	}
	if bodyHTML != "" {
		part, _ := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"text/html; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		qp := quotedprintable.NewWriter(part)
		qp.Write([]byte(bodyHTML))
		qp.Close()
	}
}

// writeBase64Wrapped writes data as base64, hard-wrapped at 76 columns per
// RFC 2045 so strict MTAs accept the message.
func writeBase64Wrapped(w io.Writer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		w.Write([]byte(encoded[i:end] + "\r\n"))
	}
}

// ---------- Internal helpers ----------

func (c *Client) sendRaw(ctx context.Context, from string, to []string, data []byte) *errx.MailError {
	var host string
	var port int
	var security string

	switch c.AuthType {
	case models.AuthPlain:
		host = c.Credentials.Host
		port = c.Credentials.Port
		security = c.Credentials.Security
	case models.AuthOAuth2:
		host = c.Oauth2.Host
		port = c.Oauth2.Port
	}

	// Normalized before it is used anywhere: brackets belong to the address,
	// not to the host, and JoinHostPort is what puts them back for an IPv6
	// literal.
	host = models.NormalizeMailHost(host)
	addr := models.MailDialAddress(host, port)
	tlsConf := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: netbind.InsecureTLS(), //nolint:gosec // MAIL_TLS_INSECURE, local dev only
		MinVersion:         tls.VersionTLS12,
	}

	// Everything after the dial gets a deadline. net/smtp sets none of its
	// own and only the connect was bounded, so a peer that stopped answering
	// without closing the connection parked the send goroutine forever: the
	// greeting, AUTH, each RCPT and the wait for the server's verdict at the
	// end of DATA all block with nothing to fail them.
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var conn net.Conn
	var err error
	// Implicit TLS (SMTPS) means the server speaks TLS from the first byte, so
	// a plaintext dial + STARTTLS never gets past the greeting. The mode is
	// the mailbox's stored choice, falling back to the port convention.
	resolved := models.ResolveSMTPSecurity(security, port)
	// The unencrypted mode is checked before the dial and again against the
	// peer we actually got, because only the second one is a fact about this
	// socket rather than about what DNS said a moment ago.
	if resolved == models.MailSecurityNone && !models.CleartextMailAllowed(host) {
		return errx.ErrMailInsecureRemoteHost
	}
	implicitTLS := resolved == models.MailSecurityTLS
	if implicitTLS {
		conn, err = netbind.TLSDialer(c.BindIP, tlsConf).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = netbind.Dialer(c.BindIP).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return errx.ErrMailServerUnreachable
	}
	defer conn.Close()
	if resolved == models.MailSecurityNone && !netbind.LoopbackPeer(conn) {
		return errx.ErrMailInsecureRemoteHost
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// Use the resolved host: c.Credentials is nil for OAuth2-configured clients.
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return errx.ErrMailServerUnreachable
	}
	defer client.Quit()

	// Announce the sender's own domain. net/smtp says "localhost" when left
	// alone, which relays read as a spam signal.
	if name := ehloName(c.Email); name != "" {
		if err := client.Hello(name); err != nil {
			return errx.ErrMailServerUnreachable
		}
	}

	// TLS is mandatory everywhere but the loopback mode, which was already
	// proved to be talking to this machine. STARTTLS is not attempted there
	// even when the relay advertises it: a self-signed certificate no client
	// can verify is exactly why the mode was chosen. The MAIL_TLS_INSECURE
	// dev knob additionally allows a server with no STARTTLS at all (the
	// local mailpit sink), never taken in production, where it is unset.
	if !implicitTLS && resolved != models.MailSecurityNone {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConf); err != nil {
				return errx.ErrMailServerUnreachable
			}
		} else if !netbind.InsecureTLS() && !c.plaintext {
			return errx.ErrMailServerUnreachable
		}
	}

	// --- Auth ---
	switch c.AuthType {
	case models.AuthPlain:
		// Negotiated, not assumed: a server that advertises only LOGIN
		// rejects a blind AUTH PLAIN, and that rejection reads exactly like a
		// wrong password, so the mailbox was deactivated over credentials
		// that were correct.
		// The normalized host, not the stored one: net/smtp records the name
		// it was handed in NewClient as the server name, and PlainAuth
		// refuses to authenticate when its own host does not match it. A
		// bracketed IPv6 literal differs from the normalized form, so the
		// stored string would fail on the address it is correct about.
		auth, aerr := NegotiateAuth(client, c.Credentials.Username, c.Credentials.Password, host)
		if aerr != nil {
			return errx.ErrMailAuthUnsupported
		}
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				// Our own refusal to authenticate over an unencrypted link,
				// raised before anything reaches the server. Retrying cannot
				// encrypt it, and calling it an outage sends the operator
				// looking at a server that is answering fine.
				if errors.Is(err, ErrSMTPCleartextAuth) {
					return errx.ErrMailCleartextAuth
				}
				// A 4xx is the server saying "not now" (rate-limited AUTH, a
				// backend it cannot reach); only a 5xx means the credentials
				// themselves are refused.
				if !permanentReply(err) {
					return errx.ErrMailServerUnreachable
				}
				return errx.ErrMailInvalidCredentials
			}
		}
	case models.AuthOAuth2:
		tk, err := c.Oauth2.Token.Token()
		if err != nil {
			var rErr *oauth2.RetrieveError
			if errors.As(err, &rErr) {
				if rErr.Response.StatusCode >= 500 {
					return errx.ErrMailServerUnreachable
				}
			}
			return errx.ErrMailAuthenticationFailed
		}

		auth := newOAuth2Auth(c.Email, tk.AccessToken)
		if err := client.Auth(auth); err != nil {
			return errx.ErrMailAuthenticationFailed
		}
	}

	if err := client.Mail(from); err != nil {
		// A refused MAIL FROM is how a blocked sender, an over-quota mailbox
		// and a relay-denied policy arrive. Retrying a 5xx never succeeds and
		// reports an outage that is not happening.
		if permanentReply(err) {
			if isDomainAuthRejection(err) {
				return errx.ErrMailDomainAuthRejected
			}
			return errx.ErrMailSendRejected(err.Error())
		}
		return errx.ErrMailServerUnreachable
	}
	for _, r := range to {
		if err := client.Rcpt(r); err != nil {
			// A domain-authentication refusal is about OUR domain, not this
			// recipient; suppressing the address would punish the wrong party.
			if isDomainAuthRejection(err) {
				return errx.ErrMailDomainAuthRejected
			}
			// A refused RCPT is a recipient problem (bad address, policy
			// rejection), not a dead server; classifying it as unreachable
			// hid rejections from bounce accounting. A 4xx is greylisting or
			// a busy server, which is worth another attempt.
			if !permanentReply(err) {
				return errx.ErrMailServerUnreachable
			}
			return errx.ErrMailRecipientRejected(err.Error())
		}
	}
	w, err := client.Data()
	if err != nil {
		return errx.ErrMailServerUnreachable
	}
	if _, err := w.Write(data); err != nil {
		return errx.ErrMailServerUnreachable
	}
	if err := w.Close(); err != nil {
		// The server's verdict on the whole message lands here, which is where
		// Microsoft returns 5.7.515. Retrying it as an outage never succeeds.
		if isDomainAuthRejection(err) {
			return errx.ErrMailDomainAuthRejected
		}
		if permanentReply(err) {
			return errx.ErrMailSendRejected(err.Error())
		}
		return errx.ErrMailServerUnreachable
	}

	return nil
}
