package config

import (
	"context"
	"os"
	"strings"
)

func (c *Config) LoadSentryDSNApi(ctx context.Context) (string, error) {
	return c.GetSecret(ctx, "SENTRY_DSN_API", "sentry_dsn/api")
}

func (c *Config) LoadSentryDSNBackend(ctx context.Context) (string, error) {
	return c.GetSecret(ctx, "SENTRY_DSN", "sentry_dsn/backend")
}

// LoadPostHogKey returns the project key server-side product analytics posts
// with. Empty means analytics is off, which is the self-host default and the
// default everywhere else: nothing is sent and no host is contacted.
func (c *Config) LoadPostHogKey(ctx context.Context) string {
	key, err := c.GetSecret(ctx, "POSTHOG_KEY", "posthog/key")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(key)
}

// PostHogHost is the capture host. Empty falls back to PostHog Cloud EU in the
// analytics package; set it to point at a self-hosted PostHog.
func PostHogHost() string {
	return strings.TrimSpace(os.Getenv("POSTHOG_HOST"))
}
