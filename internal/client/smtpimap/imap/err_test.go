package imap

import (
	"io"
	"net"
	"testing"

	"github.com/warmbly/warmbly/internal/errx"
)

// A transport error must never read as success: mapping net.ErrClosed to nil
// is what let a dropped Gmail session run as a clean "no folders" pass every
// minute for days, with nothing logged and no mail synced.
func TestHandleErrorTransportIsNotNil(t *testing.T) {
	c := &Client{}
	for _, err := range []error{net.ErrClosed, io.EOF, io.ErrUnexpectedEOF} {
		got := c.handleError(err)
		if got == nil {
			t.Fatalf("handleError(%v) = nil, want a retryable mail error", err)
		}
		if got.Code != errx.MailErrorCodeServerUnreachable {
			t.Errorf("handleError(%v).Code = %q, want %q", err, got.Code, errx.MailErrorCodeServerUnreachable)
		}
	}
	if c.handleError(nil) != nil {
		t.Error("handleError(nil) must stay nil")
	}
}
