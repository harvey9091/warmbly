package tasks

import (
	"strings"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
)

// trackingOverrideIgnored is a tracking domain that was configured but not
// used, with the sentence the campaign log records for it.
type trackingOverrideIgnored struct {
	Scope   string
	Domain  string
	Message string
}

// resolveTrackingHost picks the host open pixels and click tickets are built
// from: a VERIFIED campaign override wins, then a VERIFIED mailbox domain,
// otherwise this install's own tracking host.
//
// Only a verified override is honored. An unresolved host could point tracking
// at a target somebody else controls (SSRF-adjacent, matching the
// webhook-safety posture), and an unresolvable one is worse than no tracking:
// every link in the email becomes a ticket on a host that does not answer, so
// the recipient cannot reach the destination at all.
func resolveTrackingHost(defaultHost string, account *models.Email, campaign *models.Campaign) (string, []trackingOverrideIgnored) {
	host := defaultHost
	var ignored []trackingOverrideIgnored

	if account != nil && account.TrackingDomain != "" {
		if account.TrackingDomainVerified {
			host = account.TrackingDomain
		} else {
			ignored = append(ignored, trackingOverrideIgnored{
				Scope:   "mailbox",
				Domain:  account.TrackingDomain,
				Message: "Mailbox tracking domain is not verified; tracking through the shared host instead",
			})
		}
	}

	if campaign != nil && campaign.TrackingDomain != "" {
		if campaign.TrackingDomainVerified {
			host = campaign.TrackingDomain
		} else {
			ignored = append(ignored, trackingOverrideIgnored{
				Scope:   "campaign",
				Domain:  campaign.TrackingDomain,
				Message: "Campaign tracking domain is not verified; tracking through the mailbox default instead",
			})
		}
	}

	return host, ignored
}

// resolveOptOutOrigin is the absolute origin a recipient's unsubscribe link is
// minted on: the workspace's OWN verified tracking domain (campaign override
// first), or "" to leave it on the API origin.
//
// Deliberately narrower than resolveTrackingHost, which falls back to the
// install's shared tracking host. An opt-out has to reach something serving:
// a verified domain is a CNAME this install resolved to its own tracking
// service, so it provably does, while the shared host is only configuration
// and is absent from a core-only install. A dead click link costs a click; a
// dead opt-out costs a spam complaint.
func resolveOptOutOrigin(account *models.Email, campaign *models.Campaign) string {
	host := ""
	if account != nil && account.TrackingDomainVerified && account.TrackingDomain != "" {
		host = account.TrackingDomain
	}
	if campaign != nil && campaign.TrackingDomainVerified && campaign.TrackingDomain != "" {
		host = campaign.TrackingDomain
	}
	// http is what TrackingURL picks for a loopback or ported host, and an
	// opt-out address a recipient reads (and a provider fetches for one-click)
	// has to be https. Such a host is a dev or LAN install, where the API
	// origin is the honest place for the link.
	origin := strings.TrimSuffix(config.TrackingURL(host, ""), "/")
	if !strings.HasPrefix(origin, "https://") {
		return ""
	}
	return origin
}
