package observability

import (
	"context"
	"os"
	"strings"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/version"
)

// InitSentry configures error reporting for one Go service.
//
// It resolves the two things a service knows only at boot, the operator's DSN
// and the build it is running, and hands them to the SDK wrapper. A missing or
// empty DSN is not an error: reporting is optional in every environment,
// including prod.
func InitSentry(ctx context.Context, cfg *config.Config, service string) error {
	dsn, err := cfg.LoadSentryDSNBackend(ctx)
	if err != nil {
		dsn = ""
	}

	return errs.Init(errs.Config{
		DSN:         dsn,
		Environment: cfg.Env,
		Release:     Release(),
		Service:     service,
	})
}

// InitSentryEnv configures error reporting for a service that reads its own
// environment instead of the shared config loader. The forms service has no
// database and no secrets backend, so it has no config.Config to consult, and
// building one just to read SENTRY_DSN would give it both.
func InitSentryEnv(service string) error {
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "dev"
	}

	return errs.Init(errs.Config{
		DSN:         strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		Environment: env,
		Release:     Release(),
		Service:     service,
	})
}

// Release is the value every service tags its events with, so a stack trace in
// Sentry names the build it came from. It is the stamped release when the build
// stamped one and the short commit otherwise; an unstamped local build reports
// "dev", which is what version.String falls back to.
func Release() string {
	if v := version.String(); v != "" && v != "dev" {
		return v
	}
	if c := version.ShortCommit(); c != "" {
		return c
	}
	return "dev"
}
