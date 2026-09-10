package config

import (
	"context"
	"os"
	"strconv"
	"strings"
)

func (c *Config) LoadSentryDSNApi(ctx context.Context) (string, error) {
	return c.GetSecret(ctx, "SENTRY_DSN_API", "sentry_dsn/api")
}

func (c *Config) LoadSentryDSNBackend(ctx context.Context) (string, error) {
	return c.GetSecret(ctx, "SENTRY_DSN", "sentry_dsn/backend")
}

// LoadPostHogKey returns the project key server-side product analytics and
// error tracking post with. Empty means both are off, which is the self-host
// default and the default everywhere else: nothing is sent and no host is
// contacted.
func (c *Config) LoadPostHogKey(ctx context.Context) string {
	key, err := c.GetSecret(ctx, "POSTHOG_KEY", "posthog/key")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(key)
}

// PostHogHost is the capture host. Empty falls back to PostHog Cloud US; set it
// to point at a self-hosted PostHog.
func PostHogHost() string {
	return strings.TrimSpace(os.Getenv("POSTHOG_HOST"))
}

// PostHogErrorTracking reports whether exceptions are reported to PostHog. It
// is on whenever a key is set, because error tracking is the reason most
// instances configure one at all; POSTHOG_ERROR_TRACKING=false keeps the key
// for product analytics and sends no exceptions.
func PostHogErrorTracking() bool {
	raw := strings.TrimSpace(os.Getenv("POSTHOG_ERROR_TRACKING"))
	if raw == "" {
		return true
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return b
}
