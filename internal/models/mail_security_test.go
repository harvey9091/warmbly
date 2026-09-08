package models

import "testing"

// The resolvers are what keep mailboxes connected across the rollout: a stored
// mode wins, and an empty one has to reproduce the port-inferred behaviour the
// clients had before the column existed.
func TestResolveSMTPSecurity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		security string
		port     int
		want     string
	}{
		{"stored tls wins over port", MailSecurityTLS, 587, MailSecurityTLS},
		{"stored starttls wins over port", MailSecurityStartTLS, 465, MailSecurityStartTLS},
		{"empty infers implicit tls on 465", "", 465, MailSecurityTLS},
		{"empty infers starttls on 587", "", 587, MailSecurityStartTLS},
		{"empty infers starttls on 2525", "", 2525, MailSecurityStartTLS},
		{"garbage falls back to the port", "banana", 465, MailSecurityTLS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveSMTPSecurity(tc.security, tc.port); got != tc.want {
				t.Fatalf("ResolveSMTPSecurity(%q, %d) = %q, want %q", tc.security, tc.port, got, tc.want)
			}
		})
	}
}

func TestResolveIMAPSecurity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		security string
		port     int
		want     string
	}{
		{"stored starttls wins over port", MailSecurityStartTLS, 993, MailSecurityStartTLS},
		{"stored tls wins over port", MailSecurityTLS, 143, MailSecurityTLS},
		// Implicit TLS on anything but 143 reproduces the old always-TLS dial.
		{"empty infers implicit tls on 993", "", 993, MailSecurityTLS},
		{"empty infers implicit tls on a custom port", "", 9993, MailSecurityTLS},
		{"empty infers starttls on 143", "", 143, MailSecurityStartTLS},
		{"garbage falls back to the port", "banana", 143, MailSecurityStartTLS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveIMAPSecurity(tc.security, tc.port); got != tc.want {
				t.Fatalf("ResolveIMAPSecurity(%q, %d) = %q, want %q", tc.security, tc.port, got, tc.want)
			}
		})
	}
}

func TestValidMailSecurity(t *testing.T) {
	for _, s := range []string{MailSecurityTLS, MailSecurityStartTLS, MailSecurityNone} {
		if !ValidMailSecurity(s) {
			t.Fatalf("ValidMailSecurity(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "ssl", "TLS", "plain", "insecure"} {
		if ValidMailSecurity(s) {
			t.Fatalf("ValidMailSecurity(%q) = true, want false", s)
		}
	}
}

// "none" is the one mode that puts a password on an unencrypted socket, so it
// is never reached by inference: only a stored, explicit choice produces it.
func TestSecurityNoneIsNeverInferred(t *testing.T) {
	for _, port := range []int{25, 143, 465, 587, 993, 1025, 1143, 2525} {
		if got := ResolveSMTPSecurity("", port); got == MailSecurityNone {
			t.Errorf("ResolveSMTPSecurity(\"\", %d) inferred %q", port, got)
		}
		if got := ResolveIMAPSecurity("", port); got == MailSecurityNone {
			t.Errorf("ResolveIMAPSecurity(\"\", %d) inferred %q", port, got)
		}
	}
	// Stored, it is obeyed on any port: Proton Bridge listens on 1143/1025.
	if got := ResolveIMAPSecurity(MailSecurityNone, 1143); got != MailSecurityNone {
		t.Errorf("ResolveIMAPSecurity(none, 1143) = %q, want none", got)
	}
	if got := ResolveSMTPSecurity(MailSecurityNone, 1025); got != MailSecurityNone {
		t.Errorf("ResolveSMTPSecurity(none, 1025) = %q, want none", got)
	}
}

// The loopback rule is what makes the unencrypted mode safe, so it has to be
// exact: a name that merely looks local is not local.
func TestLoopbackMailHost(t *testing.T) {
	for _, h := range []string{"localhost", "LocalHost", " localhost ", "127.0.0.1", "127.0.1.5", "::1", "[::1]"} {
		if !LoopbackMailHost(h) {
			t.Errorf("LoopbackMailHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{
		"", "example.com", "0.0.0.0", "10.0.0.1", "192.168.1.10", "169.254.169.254",
		// Names that read as local but resolve wherever their owner points
		// them, which is the whole reason only literals are accepted.
		"localhost.example.com", "notlocalhost", "127.0.0.1.example.com",
	} {
		if LoopbackMailHost(h) {
			t.Errorf("LoopbackMailHost(%q) = true, want false", h)
		}
	}
}

// An IPv6 literal needs brackets in a host:port string. Building the address
// with fmt.Sprintf turned "::1" and 1143 into "::1:1143", which is a host with
// no port, so every dial to a mailbox on an IPv6 address failed looking like a
// dead server.
func TestMailDialAddress(t *testing.T) {
	for _, tc := range []struct {
		host string
		port int
		want string
	}{
		{"imap.example.com", 993, "imap.example.com:993"},
		{"127.0.0.1", 1143, "127.0.0.1:1143"},
		{"::1", 1143, "[::1]:1143"},
		// Already bracketed, as a user pastes it. Not double-bracketed.
		{"[::1]", 1025, "[::1]:1025"},
		{"2001:db8::5", 993, "[2001:db8::5]:993"},
		{" mail.example.com ", 587, "mail.example.com:587"},
	} {
		if got := MailDialAddress(tc.host, tc.port); got != tc.want {
			t.Errorf("MailDialAddress(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}

// The host that reaches tls.Config.ServerName and the SMTP greeting carries no
// brackets: they belong to the address, and verification against a bracketed
// literal fails on an address that is otherwise correct.
func TestNormalizeMailHost(t *testing.T) {
	for _, tc := range [][2]string{
		{"[::1]", "::1"},
		{" [2001:db8::5] ", "2001:db8::5"},
		{"127.0.0.1", "127.0.0.1"},
		{"imap.example.com", "imap.example.com"},
	} {
		if got := NormalizeMailHost(tc[0]); got != tc[1] {
			t.Errorf("NormalizeMailHost(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

// Credentials reach a worker by more than the connect form: an organization
// archive exported from a self-hosted instance carries its mailboxes, so the
// deployment half of the rule is checked where the socket is opened too.
func TestCleartextMailAllowed(t *testing.T) {
	t.Run("self-hosted", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_MODE", "self_hosted")
		if !CleartextMailAllowed("127.0.0.1") {
			t.Error("a loopback host on a self-hosted instance was refused")
		}
		if CleartextMailAllowed("imap.proton.me") {
			t.Error("a remote host was allowed")
		}
	})
	t.Run("hosted", func(t *testing.T) {
		t.Setenv("DEPLOYMENT_MODE", "cloud")
		if CleartextMailAllowed("127.0.0.1") {
			t.Error("the worker's own loopback was allowed on the hosted product")
		}
	})
}
