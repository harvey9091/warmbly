package config

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// AppBaseURL is the dashboard origin every emailed link is built from.
//
// This used to be a hardcoded https://app.warmbly.com, which meant a
// self-hosted deployment mailed its users a reset link pointing at someone
// else's dashboard, carrying a live reset token signed with the self-host's own
// AUTH_SECRET. APP_URL is the documented variable; FRONTEND_BASE_URL is the
// older name and stays supported so existing deployments keep working.
func AppBaseURL() string {
	for _, key := range []string{"APP_URL", "FRONTEND_BASE_URL"} {
		if v := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); v != "" {
			return v
		}
	}
	return "https://app.warmbly.com"
}

// WebsocketURL is the realtime gateway clients connect to. It is deployment
// configuration rather than a secret, which is why GET /v1/auth/config serves
// it: a CLI or a developer client cannot otherwise find the socket on a
// self-hosted instance, where the host layout is whatever the operator chose.
func WebsocketURL() string {
	v := strings.TrimRight(strings.TrimSpace(os.Getenv("WEBSOCKET_URL")), "/")
	if v == "" {
		return ""
	}
	// The variable is written three ways in the wild: a bare host, the Phoenix
	// socket mount (".../socket"), and the full transport endpoint. Clients
	// dial what this returns, so all three normalise to the last one. Matching
	// on a "/socket" substring instead of the suffix left ".../socket"
	// untouched, which is not a websocket endpoint.
	switch {
	case strings.HasSuffix(v, "/socket/websocket"):
	case strings.HasSuffix(v, "/socket"):
		v += "/websocket"
	default:
		v += "/socket/websocket"
	}
	return v
}

func GetPasswordResetURL(sessionToken string) string {
	return AppBaseURL() + "/auth/reset-password/confirm?session=" + url.QueryEscape(sessionToken)
}

// GetInviteURL is the team-invitation accept link. Same reasoning as the reset
// URL: the dashboard's own copy-link button already used the browser origin, so
// only the emailed variant was broken on self-host.
func GetInviteURL(token string) string {
	return AppBaseURL() + "/invite?token=" + url.QueryEscape(token)
}

// FormsBaseURL is the origin hosted form pages are served from: FORMS_DOMAIN,
// the host routed to the forms service (cmd/forms). Empty when unset — the
// pages do not live on the API origin, so there is nothing to fall back to,
// and a share link pointing at the wrong process is worse than none.
func FormsBaseURL() string {
	host := NormalizeTrackingHost(os.Getenv("FORMS_DOMAIN"))
	if host == "" {
		return ""
	}
	return formsScheme(host) + "://" + host
}

// formsScheme is https except where TLS cannot be terminated: a form page on a
// loopback or private-network host is a development or LAN install. The port is
// deliberately not a signal, because an install can terminate TLS on any port
// and inferring http from one handed an https deployment http:// share links.
func formsScheme(host string) string {
	name := hostWithoutPort(NormalizeTrackingHost(host))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") {
		return "http"
	}
	if ip := net.ParseIP(strings.Trim(name, "[]")); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return "http"
	}
	return "https"
}

// FormsHostname is the bare host this install serves forms on. It is the
// CNAME target a customer points their own forms subdomain at, so it never
// carries a port.
func FormsHostname() string {
	return hostWithoutPort(NormalizeTrackingHost(FormsBaseURL()))
}

// FormsURLHost is the shared host form URLs are built on. Unlike the CNAME
// target it keeps the port, because dropping it points every share link and
// embed on a ported install at nothing.
func FormsURLHost() string {
	return NormalizeTrackingHost(FormsBaseURL())
}

// FormURLOn builds the hosted page URL on a specific host, which is how a
// verified custom forms domain replaces the shared one. An empty host falls
// back to this install's own forms base.
func FormURLOn(host, publicID string) string {
	host = NormalizeTrackingHost(host)
	if host == "" {
		return GetFormURL(publicID)
	}
	return formsScheme(host) + "://" + host + "/f/" + url.PathEscape(publicID)
}

// GetFormURL is the hosted page for one form; empty when no base is known.
func GetFormURL(publicID string) string {
	base := FormsBaseURL()
	if base == "" {
		return ""
	}
	return base + "/f/" + url.PathEscape(publicID)
}
