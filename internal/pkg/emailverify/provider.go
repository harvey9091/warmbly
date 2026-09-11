package emailverify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrProviderKey     = errors.New("verification provider rejected the API key")
	ErrProviderCredits = errors.New("verification account has no allowance or credits left")
	// ErrProviderUnconfirmed is a key that is valid but belongs to an account
	// whose owner has not confirmed their email address yet.
	ErrProviderUnconfirmed = fmt.Errorf("verification account is not confirmed: %w", ErrProviderKey)
)

// ProviderClient exposes paid verification and a non-billable account check.
type ProviderClient interface {
	Check(context.Context, string) (Result, error)
	// Account returns nil for the balance when the provider does not expose it.
	Account(context.Context) (*int, error)
	// ObservesBalance reports whether Account can tell an exhausted account
	// from a healthy one. When it cannot, only a real check reveals exhaustion,
	// so a recorded one has to be held rather than re-derived from Account.
	ObservesBalance() bool
}

// refuseInsecureRedirect stops a redirect that downgrades the scheme. Go keeps
// the Authorization header across a redirect that stays on the same host, so an
// http destination would put the API key on the wire in the clear.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Scheme != "https" && len(via) > 0 && via[0].URL.Scheme == "https" {
		return fmt.Errorf("refusing a redirect from https to %s", req.URL.Scheme)
	}
	return nil
}
