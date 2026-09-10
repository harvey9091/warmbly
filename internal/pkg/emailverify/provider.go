package emailverify

import (
	"context"
	"errors"
)

var (
	ErrProviderKey     = errors.New("verification provider rejected the API key")
	ErrProviderCredits = errors.New("verification account has no allowance or credits left")
)

// ProviderClient exposes paid verification and a non-billable account check.
type ProviderClient interface {
	Check(context.Context, string) (Result, error)
	// Account returns nil for the balance when the provider does not expose it.
	Account(context.Context) (*int, error)
}
