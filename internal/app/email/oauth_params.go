package email

import (
	"github.com/warmbly/warmbly/internal/models"
	"golang.org/x/oauth2"
)

// authCodeOptions returns the authorization-request parameters for a provider.
// loginHint preselects a mailbox on a reconnect and is empty on a first connect.
//
// The two providers disagree about what it takes to be issued a refresh token,
// and asking for the wrong one is not free:
//
//   - Google issues one only when access_type=offline is set, and re-issues one
//     on a repeat authorization only when the consent screen is forced.
//   - Microsoft issues one off the offline_access scope alone, so prompt=consent
//     buys nothing there and costs a lot: Entra ID re-runs the consent
//     eligibility check on every sign-in instead of honouring the grant already
//     on the tenant, and refuses every non-admin with AADSTS90095 even when
//     tenant-wide admin consent was granted (issue #409). prompt=select_account
//     keeps the account picker, which is what stops a signed-in browser
//     silently connecting the wrong mailbox, without that check.
func authCodeOptions(provider models.InboxProvider, loginHint string) []oauth2.AuthCodeOption {
	var opts []oauth2.AuthCodeOption
	if provider == models.InboxProviderOutlook {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	} else {
		opts = append(opts, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	}
	if loginHint != "" {
		// Preselect the mailbox being renewed in the provider's picker.
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", loginHint))
	}
	return opts
}
