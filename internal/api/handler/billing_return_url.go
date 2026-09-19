package handler

import (
	"strings"

	"github.com/warmbly/warmbly/internal/config"
)

// billingReturnURL turns a client-supplied Stripe return URL into one that can
// only point back at this instance's dashboard.
//
// Stripe redirects the customer to whatever success_url, cancel_url or
// return_url the session was created with, so accepting the value verbatim
// made every billing endpoint an open redirect laundered through
// checkout.stripe.com, which is about as trustworthy a hop as a phishing link
// can get. Every shipped client already sends a same-origin path, so pinning
// the origin here changes nothing a real client does.
//
// A path is kept, so "which page did you come from" still works. Anything else
// falls back to the dashboard root.
func billingReturnURL(raw, fallbackPath string) string {
	base := strings.TrimRight(config.AppBaseURL(), "/")
	raw = strings.TrimSpace(raw)

	// A deployment that never configured its own address has no origin to pin
	// to, and a relative fallback is not a URL Stripe will accept: returning one
	// would take checkout out entirely. Hand back what the caller sent, which is
	// the behaviour before this function existed.
	if base == "" {
		return raw
	}

	fallback := base + fallbackPath
	if raw == "" {
		return fallback
	}
	// A relative path is the common case and needs no parsing beyond refusing
	// the "//evil.example" form, which a browser reads as a protocol-relative
	// absolute URL.
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return base + raw
	}
	if base != "" && strings.HasPrefix(raw, base+"/") {
		return raw
	}
	if raw == base {
		return raw
	}
	return fallback
}
