package email

import (
	"testing"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"golang.org/x/oauth2"
)

// config.LoadOauth2Inbox always returns a non-nil config whose fields are empty
// when the variables are unset, so a nil check alone reports every provider as
// available and the flow fails later with an opaque error from the provider.
func TestOAuthConfigFor_UnconfiguredProviderIsReported(t *testing.T) {
	svc := &emailService{oauthInbox: &config.Oauth2Inbox{
		Google:  &oauth2.Config{},
		Outlook: &oauth2.Config{},
	}}

	for _, tc := range []struct {
		provider models.InboxProvider
		want     *errx.Error
	}{
		{models.InboxProviderGoogle, errx.ErrEmailOnboardGoogleNotConfigured},
		{models.InboxProviderOutlook, errx.ErrEmailOnboardOutlookNotConfigured},
	} {
		cfg, err := svc.oauthConfigFor(tc.provider)
		if cfg != nil {
			t.Errorf("%s: expected no config, got one", tc.provider)
		}
		if err != tc.want {
			t.Errorf("%s: expected %v, got %v", tc.provider, tc.want, err)
		}
		if err != nil && err.Identifier != "mailbox_provider_not_configured" {
			t.Errorf("%s: clients branch on this identifier, got %q", tc.provider, err.Identifier)
		}
	}
}

// A half-set credential is still unusable, and silently building an OAuth URL
// from it sends the user to a provider error page instead of telling them what
// to fix.
func TestOAuthConfigFor_PartialCredentialsAreNotConfigured(t *testing.T) {
	svc := &emailService{oauthInbox: &config.Oauth2Inbox{
		Google:  &oauth2.Config{ClientID: "id-without-secret"},
		Outlook: &oauth2.Config{ClientSecret: "secret-without-id"},
	}}

	if _, err := svc.oauthConfigFor(models.InboxProviderGoogle); err != errx.ErrEmailOnboardGoogleNotConfigured {
		t.Errorf("google with no secret should be unconfigured, got %v", err)
	}
	if _, err := svc.oauthConfigFor(models.InboxProviderOutlook); err != errx.ErrEmailOnboardOutlookNotConfigured {
		t.Errorf("outlook with no client id should be unconfigured, got %v", err)
	}
}

func TestOAuthConfigFor_ConfiguredProviderIsReturned(t *testing.T) {
	google := &oauth2.Config{ClientID: "id", ClientSecret: "secret"}
	svc := &emailService{oauthInbox: &config.Oauth2Inbox{
		Google:  google,
		Outlook: &oauth2.Config{},
	}}

	cfg, err := svc.oauthConfigFor(models.InboxProviderGoogle)
	if err != nil {
		t.Fatalf("configured google should be returned, got %v", err)
	}
	if cfg != google {
		t.Error("expected the configured google client")
	}

	// One provider being configured must not make the other appear available.
	if _, err := svc.oauthConfigFor(models.InboxProviderOutlook); err != errx.ErrEmailOnboardOutlookNotConfigured {
		t.Errorf("outlook should still be unconfigured, got %v", err)
	}
}

// The unencrypted mailbox mode has two gates and they answer different
// questions: whether this deployment can host a local relay at all, and
// whether the host given is that relay. A user who gets the wrong one back is
// sent to fix the wrong thing.
func TestValidateMailSecurity_CleartextIsLoopbackOnlyAndSelfHostOnly(t *testing.T) {
	svc := func(host, security string) *models.Service {
		return &models.Service{Host: host, Port: 1143, Security: security}
	}
	tls := svc("imap.example.com", models.MailSecurityTLS)

	for _, tc := range []struct {
		name       string
		deployment string
		smtp, imap *models.Service
		want       *errx.Error
	}{
		{"self-host, loopback imap", "self_hosted", svc("smtp.example.com", models.MailSecurityStartTLS), svc("127.0.0.1", models.MailSecurityNone), nil},
		{"self-host, localhost smtp", "self_hosted", svc("localhost", models.MailSecurityNone), tls, nil},
		{"self-host, ipv6 loopback", "self_hosted", svc("::1", models.MailSecurityNone), tls, nil},
		{"self-host, remote imap", "self_hosted", svc("smtp.example.com", models.MailSecurityTLS), svc("imap.proton.me", models.MailSecurityNone), errx.ErrEmailIMAPSecurityNotLocal},
		{"self-host, remote smtp", "self_hosted", svc("mail.example.com", models.MailSecurityNone), tls, errx.ErrEmailSMTPSecurityNotLocal},
		// Hosted: the worker is not the customer's machine, so a loopback
		// address there is the worker's own and the mode is refused outright.
		{"hosted, loopback smtp", "cloud", svc("127.0.0.1", models.MailSecurityNone), tls, errx.ErrEmailSMTPSecurityHosted},
		{"hosted, loopback imap", "cloud", svc("smtp.example.com", models.MailSecurityTLS), svc("localhost", models.MailSecurityNone), errx.ErrEmailIMAPSecurityHosted},
		{"unknown mode still rejected", "self_hosted", svc("localhost", "plain"), tls, errx.ErrEmailSMTPSecurity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DEPLOYMENT_MODE", tc.deployment)
			got := validateMailSecurity(tc.smtp, tc.imap)
			if got != tc.want {
				t.Fatalf("validateMailSecurity() = %v, want %v", got, tc.want)
			}
		})
	}
}
