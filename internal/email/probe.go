package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"strings"
	"unicode"

	"github.com/warmbly/warmbly/internal/models"
)

// ProbeResult is the outcome of one credential probe. Reason is one of the
// models.MailProbe* values; Detail is the server's reply or the dial error.
type ProbeResult struct {
	OK     bool
	Reason string
	Detail string
}

// Leg converts the result to the wire shape the worker publishes.
func (r ProbeResult) Leg() models.EmailValidationLeg {
	return models.EmailValidationLeg{OK: r.OK, Reason: r.Reason, Detail: r.Detail}
}

func probeOK() ProbeResult { return ProbeResult{OK: true} }

// probeFail classifies err. A deadline anywhere wins over the given reason,
// because a server that never answered has not refused anything.
func probeFail(ctx context.Context, reason string, err error) ProbeResult {
	if timedOut(ctx, err) {
		reason = models.MailProbeTimeout
	}
	return ProbeResult{Reason: reason, Detail: probeDetail(err)}
}

func probeFailText(reason, detail string) ProbeResult {
	return ProbeResult{Reason: reason, Detail: detail}
}

// timedOut reports whether err, or the context it ran under, is a deadline.
func timedOut(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}

// dialReason separates a failed handshake from a failed connection: an implicit
// TLS dial returns both through one error.
func dialReason(err error) string {
	if isTLSError(err) {
		return models.MailProbeTLS
	}
	return models.MailProbeUnreachable
}

func isTLSError(err error) bool {
	var certErr *tls.CertificateVerificationError
	var hdrErr tls.RecordHeaderError
	var alert tls.AlertError
	var unknownCA x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	switch {
	case errors.As(err, &certErr), errors.As(err, &hdrErr), errors.As(err, &alert),
		errors.As(err, &unknownCA), errors.As(err, &hostErr), errors.As(err, &invalid):
		return true
	}
	return err != nil && strings.HasPrefix(err.Error(), "tls:")
}

// probeDetail flattens an error into one printable line, capped, so a server
// reply can be shown at the form without dragging a protocol dump along.
func probeDetail(err error) string {
	if err == nil {
		return ""
	}
	const maxDetail = 240
	var b strings.Builder
	space := false
	for _, r := range err.Error() {
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
		if b.Len() >= maxDetail {
			break
		}
	}
	return b.String()
}
