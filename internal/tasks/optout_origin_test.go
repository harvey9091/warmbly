package tasks

import (
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

func TestResolveOptOutOrigin(t *testing.T) {
	verifiedMailbox := &models.Email{TrackingDomain: "t.acme.com", TrackingDomainVerified: true}
	pendingMailbox := &models.Email{TrackingDomain: "t.acme.com"}

	cases := []struct {
		name     string
		instance string
		account  *models.Email
		campaign *models.Campaign
		want     string
	}{
		{name: "no custom domain stays on the API origin", instance: "track.warmbly.com", want: ""},
		{name: "verified mailbox domain", instance: "track.warmbly.com", account: verifiedMailbox, want: "https://t.acme.com"},
		{
			name:     "verified campaign override wins",
			instance: "track.warmbly.com",
			account:  verifiedMailbox,
			campaign: &models.Campaign{TrackingDomain: "t.promo.acme.com", TrackingDomainVerified: true},
			want:     "https://t.promo.acme.com",
		},
		{
			name:     "unverified campaign override falls back to the mailbox",
			instance: "track.warmbly.com",
			account:  verifiedMailbox,
			campaign: &models.Campaign{TrackingDomain: "t.promo.acme.com"},
			want:     "https://t.acme.com",
		},
		{name: "unverified mailbox domain is never used", instance: "track.warmbly.com", account: pendingMailbox, want: ""},
		{
			// One-click needs https, and the API origin is the honest place
			// for a link on a host that cannot terminate TLS.
			name:    "a ported dev host stays on the API origin",
			account: &models.Email{TrackingDomain: "localhost:3000", TrackingDomainVerified: true},
			want:    "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TRACKING_DOMAIN", c.instance)
			if got := resolveOptOutOrigin(c.account, c.campaign); got != c.want {
				t.Errorf("resolveOptOutOrigin = %q, want %q", got, c.want)
			}
		})
	}
}

// The click wrapper recognises an opt-out by its path, so a link minted on a
// workspace's own tracking domain must still be left alone: wrapping it would
// count the opt-out as a click and bounce the recipient through a redirect.
func TestOptOutOnTrackingDomainIsNotWrapped(t *testing.T) {
	const host = "t.acme.com"
	link := "https://" + host + "/unsubscribe/abc123"
	body := `<p><a href="https://acme.com/pricing">pricing</a> <a href="` + link + `">unsubscribe</a></p>`

	out, minted := TrackLinks(body, LinkTracking{
		TaskID:         uuid.New(),
		CampaignID:     uuid.New(),
		TrackingDomain: host,
		Wrap:           true,
	})
	if !contains(out, `href="`+link+`"`) {
		t.Fatalf("opt-out link was rewritten: %s", out)
	}
	for _, m := range minted {
		if m.Destination == link {
			t.Fatal("a click ticket was minted for the opt-out link")
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
