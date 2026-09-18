package smtp

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// SERVER_UNREACHABLE was returned as one bare sentinel from a dozen points in
// the send, so a refused dial, a timeout, a rejected EHLO and a failed TLS
// handshake were indistinguishable from the outside. The code is what every
// caller switches on and must not move; the message is what makes the failure
// diagnosable (#574).
func TestUnreachableNamesTheStepThatFailed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		at     string
		reply  string
		stage  string
		detail string
	}{
		{"refused auth", "AUTH", "454 4.7.0 temporary authentication failure", "auth", "454"},
		{"deferred sender", "MAIL", "451 4.7.1 try again later", "mail from", "451"},
		{"greylisted recipient", "RCPT", "450 4.2.0 greylisted", "rcpt to", "450"},
		{"deferred message", "DATA-END", "451 4.3.0 try later", "message accept", "451"},
	} {
		srv := newRejectingServer(t, "LOGIN", map[string]string{tc.at: tc.reply})
		host, port := srv.addr()
		c := newTestClient(host, port)

		err := c.sendRaw(t.Context(), "sender@warmbly.test", []string{"to@example.test"}, []byte("Subject: hi\r\n\r\nbody\r\n"))
		if err == nil {
			t.Errorf("%s: send succeeded against %q", tc.name, tc.reply)
			continue
		}
		if string(err.Code) != "SERVER_UNREACHABLE" {
			t.Errorf("%s: code = %q, want SERVER_UNREACHABLE", tc.name, err.Code)
		}
		if !strings.Contains(err.Message, tc.stage) {
			t.Errorf("%s: message %q does not name the step %q", tc.name, err.Message, tc.stage)
		}
		if !strings.Contains(err.Message, tc.detail) {
			t.Errorf("%s: message %q dropped the server's own reply %q", tc.name, err.Message, tc.detail)
		}
	}
}

// A closed port is the failure the reporter of #574 spent an evening on: the
// message has to say the dial itself never connected, and to which address.
func TestUnreachableNamesARefusedDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, r := range portStr {
		port = port*10 + int(r-'0')
	}
	// Closed before anything dials it, so the connect is refused rather than queued.
	_ = ln.Close()

	c := newTestClient(host, port)
	mailErr := c.sendRaw(t.Context(), "sender@warmbly.test", []string{"to@example.test"}, []byte("Subject: hi\r\n\r\nbody\r\n"))
	if mailErr == nil {
		t.Fatal("send succeeded against a closed port")
	}
	if string(mailErr.Code) != "SERVER_UNREACHABLE" {
		t.Fatalf("code = %q, want SERVER_UNREACHABLE", mailErr.Code)
	}
	for _, want := range []string{"dial", portStr, "refused"} {
		if !strings.Contains(mailErr.Message, want) {
			t.Errorf("message %q does not mention %q", mailErr.Message, want)
		}
	}
}

// The other half of the same complaint: a server that offers no encrypted
// upgrade is refused by us, not by the network, and the message has to say so
// instead of reading as an outage at an address that is answering fine.
func TestUnreachableSaysWhenTheServerOffersNoSTARTTLS(t *testing.T) {
	srv := newFakeServer(t, "LOGIN")
	host, port := srv.addr()
	c := newTestClient(host, port)
	c.plaintext = false

	err := c.sendRaw(t.Context(), "sender@warmbly.test", []string{"to@example.test"}, []byte("Subject: hi\r\n\r\nbody\r\n"))
	if err == nil {
		t.Fatal("sent over a connection that was never encrypted")
	}
	if string(err.Code) != "SERVER_UNREACHABLE" {
		t.Fatalf("code = %q, want SERVER_UNREACHABLE", err.Code)
	}
	if !strings.Contains(err.Message, "starttls") || !strings.Contains(err.Message, "does not offer STARTTLS") {
		t.Errorf("message %q does not explain the missing STARTTLS", err.Message)
	}
}

// The stored security mode decides the dial, so an implicit-TLS mailbox (465)
// reports its handshake, not a plaintext greeting nobody attempted.
func TestUnreachableNamesTheDialForImplicitTLS(t *testing.T) {
	srv := newFakeServer(t, "LOGIN")
	host, port := srv.addr()
	c := newTestClient(host, port)
	c.Credentials.Security = models.MailSecurityTLS

	err := c.sendRaw(t.Context(), "sender@warmbly.test", []string{"to@example.test"}, []byte("Subject: hi\r\n\r\nbody\r\n"))
	if err == nil {
		t.Fatal("a TLS handshake succeeded against a plaintext server")
	}
	if !strings.Contains(err.Message, "dial") {
		t.Errorf("message %q does not name the dial, where implicit TLS fails", err.Message)
	}
}

// dialCause keeps the syscall failure and drops the addresses around it. The
// local one is our own egress IP (WORKER_BIND_IP), which the mailbox's owner
// has no use for and no business reading; the remote one is already in the
// stage, so repeating it only made the message harder to read.
func TestDialCauseKeepsTheFailureAndDropsTheAddresses(t *testing.T) {
	op := &net.OpError{
		Op:     "dial",
		Net:    "tcp",
		Source: &net.TCPAddr{IP: net.ParseIP("10.1.2.3")},
		Addr:   &net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 465},
		Err:    errors.New("bind: cannot assign requested address"),
	}
	got := dialCause(op).Error()
	for _, unwanted := range []string{"10.1.2.3", "203.0.113.9"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q still carries the address %s", got, unwanted)
		}
	}
	if !strings.Contains(got, "cannot assign requested address") {
		t.Errorf("%q dropped the failure itself", got)
	}
}

func TestDialCausePassesThroughAnythingElse(t *testing.T) {
	plain := errors.New("no such host")
	if got := dialCause(plain); got != plain {
		t.Fatalf("dialCause rewrote a non-OpError: %v", got)
	}
}
