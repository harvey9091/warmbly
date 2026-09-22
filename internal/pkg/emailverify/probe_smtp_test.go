package emailverify

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMX is a minimal SMTP server that answers RCPT TO with whatever the test
// tells it to. It exists to reproduce the exact wire behaviour behind issue
// #200: Postfix runs its HELO/sender restrictions at RCPT time
// (smtpd_delay_reject defaults on), so a rejected greeting arrives as a 5xx
// reply to RCPT rather than to HELO.
type fakeMX struct {
	ln       net.Listener
	rcptRepl string

	mu    sync.Mutex
	helos []string
}

func newFakeMX(t *testing.T, rcptReply string) *fakeMX {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeMX{ln: ln, rcptRepl: rcptReply}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeMX) port() string {
	_, p, _ := net.SplitHostPort(f.ln.Addr().String())
	return p
}

func (f *fakeMX) greetings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.helos...)
}

func (f *fakeMX) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			br := bufio.NewReader(c)
			io := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
			io("220 fake.mx ESMTP")
			for {
				line, err := br.ReadString('\n')
				if err != nil {
					return
				}
				cmd := strings.ToUpper(strings.TrimSpace(line))
				switch {
				case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
					f.mu.Lock()
					f.helos = append(f.helos, strings.TrimSpace(line))
					f.mu.Unlock()
					// Accepted here even when the name is unacceptable; the
					// rejection is deferred to RCPT, exactly like Postfix.
					io("250-fake.mx")
					io("250 SIZE 10240000")
				case strings.HasPrefix(cmd, "MAIL FROM"):
					io("250 2.1.0 Ok")
				case strings.HasPrefix(cmd, "RCPT TO"):
					io(f.rcptRepl)
				case strings.HasPrefix(cmd, "QUIT"):
					io("221 2.0.0 Bye")
					return
				default:
					io("250 2.0.0 Ok")
				}
			}
		}(conn)
	}
}

func testVerifier(t *testing.T, cfg Config, port string) *SMTPVerifier {
	t.Helper()
	v := New(cfg)
	v.smtpPort = port
	// The fake MX listens on loopback, which the production guard refuses.
	v.allowPrivateMX = true
	return v
}

// TestProbeTreatsDeferredHeloRejectionAsUnknown is issue #200 on the wire: the
// server rejects the greeting at RCPT time, and the prober must NOT read that
// as a dead mailbox.
func TestProbeTreatsDeferredHeloRejectionAsUnknown(t *testing.T) {
	mx := newFakeMX(t, "504 5.5.2 <localhost>: Helo command rejected: need fully-qualified hostname")
	v := testVerifier(t, Config{HeloHost: "verify.warmbly.com"}, mx.port())

	got := v.probe(context.Background(), "127.0.0.1", "real.person", "customer.com")
	if got.outcome != probeUnknown {
		t.Fatalf("outcome = %v (reason %q), want probeUnknown: a rejected greeting says nothing about the address",
			got.outcome, got.reason)
	}
	if !strings.Contains(got.reason, "not judged") {
		t.Errorf("reason %q should say the address was never judged", got.reason)
	}
}

// A real recipient rejection must still be caught, or the feature stops
// protecting the sending reputation it exists for.
func TestProbeStillCatchesUnknownMailbox(t *testing.T) {
	mx := newFakeMX(t, "550 5.1.1 <a@customer.com>: Recipient address rejected: User unknown")
	v := testVerifier(t, Config{HeloHost: "verify.warmbly.com"}, mx.port())

	got := v.probe(context.Background(), "127.0.0.1", "nobody", "customer.com")
	if got.outcome != probeRejected {
		t.Fatalf("outcome = %v (reason %q), want probeRejected", got.outcome, got.reason)
	}
}

// The probe must announce the configured FQDN, never a bare name.
func TestProbeAnnouncesConfiguredFQDN(t *testing.T) {
	mx := newFakeMX(t, "250 2.1.5 Ok")
	v := testVerifier(t, Config{HeloHost: "verify.warmbly.com"}, mx.port())

	v.probe(context.Background(), "127.0.0.1", "real.person", "customer.com")
	greetings := mx.greetings()
	if len(greetings) == 0 {
		t.Fatal("the probe never greeted the server")
	}
	for _, g := range greetings {
		if strings.Contains(strings.ToLower(g), "localhost") {
			t.Fatalf("greeted with %q; announcing localhost is what got sessions rejected", g)
		}
		if !strings.Contains(g, "verify.warmbly.com") {
			t.Fatalf("greeted with %q, want the configured FQDN", g)
		}
	}
}

// Without a usable HELO identity the verifier must decline to probe at all
// rather than dial out and collect a session rejection it would misread.
func TestVerifyRefusesToProbeWithoutUsableHeloHost(t *testing.T) {
	for _, helo := range []string{"", "localhost", "warmbly", "box.local"} {
		v := New(Config{HeloHost: helo})
		// A domain that certainly has MX records, so the probe is the only
		// thing that could produce a verdict.
		res := v.Verify(context.Background(), "someone@gmail.com")
		if res.Status != StatusUnknown {
			t.Fatalf("HeloHost %q produced status %q (%s); an unconfigured verifier must stay unknown",
				helo, res.Status, res.Reason)
		}
		if strings.Contains(res.Reason, "mx lookup failed") {
			continue // no DNS on this runner; "unknown" is already the assertion that matters
		}
		if !strings.Contains(res.Reason, "EMAIL_VERIFY_HELO_HOST") {
			t.Errorf("HeloHost %q: reason %q should tell the operator what to set", helo, res.Reason)
		}
	}
}

// Syntax and MX verdicts do not depend on the probe, so they must keep working
// on an unconfigured instance.
func TestVerifyKeepsSyntaxVerdictWithoutHeloHost(t *testing.T) {
	v := New(Config{})
	if res := v.Verify(context.Background(), "not-an-address"); res.Status != StatusInvalid {
		t.Fatalf("status = %q, want invalid for a malformed address", res.Status)
	}
}

// withDefaults must never invent a greeting name.
func TestConfigDefaultsDoNotInventAHeloHost(t *testing.T) {
	c := Config{}.withDefaults()
	if c.HeloHost != "" {
		t.Fatalf("HeloHost defaulted to %q; guessing a name is what caused issue #200", c.HeloHost)
	}
	if c.MailFrom != "" {
		t.Fatalf("MailFrom defaulted to %q with no usable host", c.MailFrom)
	}
	c2 := Config{HeloHost: "Verify.Warmbly.com."}.withDefaults()
	if c2.HeloHost != "verify.warmbly.com" {
		t.Fatalf("HeloHost = %q, want the normalized name", c2.HeloHost)
	}
	if c2.MailFrom != "verify@verify.warmbly.com" {
		t.Fatalf("MailFrom = %q, want it derived from the host", c2.MailFrom)
	}
}

// TestVerifyEndToEndAgainstDeferredHeloRejection drives the whole pipeline --
// syntax, MX, probe, classification -- against a server that behaves like the
// one in issue #200. The address is at "localhost", so the implicit-MX fallback
// resolves it to 127.0.0.1 without touching DNS.
func TestVerifyEndToEndAgainstDeferredHeloRejection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rcptReply string
		want      Status
	}{
		{"probe rejected by the server's helo policy", "504 5.5.2 <localhost>: Helo command rejected: need fully-qualified hostname", StatusUnknown},
		{"connecting host blocklisted", "554 5.7.1 Service unavailable; Client host blocked using zen.spamhaus.org", StatusUnknown},
		{"the mailbox really is gone", "550 5.1.1 <a@localhost>: Recipient address rejected: User unknown", StatusInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mx := newFakeMX(t, tc.rcptReply)
			v := testVerifier(t, Config{HeloHost: "verify.warmbly.com"}, mx.port())

			res := v.Verify(context.Background(), "real.person@localhost")
			if !res.HasMX {
				t.Skip("this runner does not resolve localhost to an implicit MX")
			}
			if res.Status != tc.want {
				t.Fatalf("Verify status = %q (%s), want %q", res.Status, res.Reason, tc.want)
			}
		})
	}
}
