// Package errs is the only place the Sentry SDK is called from.
//
// Every service, job and repository reports through these functions, so the
// vendor import stays in one file: turning reporting off, attaching a tag to
// every event, or swapping the backend is a change here rather than across
// ninety files. Nothing here needs Init to have run; an uninitialised SDK
// drops the event, which is what a deployment with no DSN wants.
package errs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// Config is what a service knows about itself at boot.
type Config struct {
	// DSN is empty when the operator configured no error reporting. Events are
	// then summarised to the local log instead of being sent anywhere.
	DSN string
	// Environment is the deployment label (dev, staging, prod).
	Environment string
	// Release identifies the build, so a stack trace can be tied to a commit.
	Release string
	// Service names the process (backend, consumer, worker, forms).
	Service string
}

// Init configures the SDK for one process. Safe to call once per process.
func Init(cfg Config) error {
	options := sentry.ClientOptions{
		SendDefaultPII: true,
		Environment:    cfg.Environment,
		Release:        cfg.Release,
		ServerName:     cfg.Service,
	}

	// A DSN is used when one is configured, in any environment. Error reporting
	// is the operator's choice, not a requirement of the software: demanding a
	// Sentry account to run APP_ENV=prod made a self-hosted deployment fail to
	// boot over a service it never asked for.
	if cfg.DSN != "" {
		options.Dsn = cfg.DSN
	} else {
		if cfg.Environment == "prod" {
			log.Printf("Sentry is not configured (no SENTRY_DSN); errors are logged locally only.")
		}
		options.BeforeSend = func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			log.Printf("[sentry-local][%s][%s] %s", cfg.Service, event.Level, summarize(event))
			return event
		}
	}

	if err := sentry.Init(options); err != nil {
		return fmt.Errorf("init sentry: %w", err)
	}
	return nil
}

// Option decorates the scope one event is reported on. Callers build these with
// Tag and Extra so they never name a Sentry type themselves.
type Option func(*sentry.Scope)

// Tag adds an indexed key/value pair, searchable and groupable in Sentry.
func Tag(key, value string) Option {
	return func(scope *sentry.Scope) { scope.SetTag(key, value) }
}

// Extra adds an unindexed value, for detail that is worth reading on the event
// but not worth searching by.
func Extra(key string, value any) Option {
	return func(scope *sentry.Scope) { scope.SetExtra(key, value) }
}

// CaptureException reports err on the process-wide hub.
func CaptureException(err error, opts ...Option) {
	capture(sentry.CurrentHub(), opts, func(hub *sentry.Hub) { hub.CaptureException(err) })
}

// CaptureExceptionContext reports err on the hub carried by ctx when there is
// one, so a request's breadcrumbs and scope travel with the event, and on the
// process-wide hub otherwise.
func CaptureExceptionContext(ctx context.Context, err error, opts ...Option) {
	capture(hubFrom(ctx), opts, func(hub *sentry.Hub) { hub.CaptureException(err) })
}

// CaptureMessage reports a message with no error attached.
func CaptureMessage(message string, opts ...Option) {
	capture(sentry.CurrentHub(), opts, func(hub *sentry.Hub) { hub.CaptureMessage(message) })
}

// CaptureMessageContext is CaptureMessage on ctx's hub.
func CaptureMessageContext(ctx context.Context, message string, opts ...Option) {
	capture(hubFrom(ctx), opts, func(hub *sentry.Hub) { hub.CaptureMessage(message) })
}

// fatalFlushTimeout is how long a process about to exit waits for its last
// event. Short enough not to stall a crash loop, long enough for one POST.
const fatalFlushTimeout = 2 * time.Second

// CaptureFatal reports err and waits for it to be sent. Use it instead of
// CaptureException wherever the next statement ends the process: the SDK sends
// in the background and os.Exit does not wait for it, so a boot failure — the
// error most worth having — was the one that never arrived.
func CaptureFatal(err error, opts ...Option) {
	CaptureException(err, opts...)
	Flush(fatalFlushTimeout)
}

// Recover reports a value from recover(). Call it inside the deferred function
// that recovered, not after.
func Recover(r any) {
	sentry.CurrentHub().Recover(r)
}

// RecoverContext is Recover on ctx's hub.
func RecoverContext(ctx context.Context, r any) {
	hubFrom(ctx).RecoverWithContext(ctx, r)
}

// Flush waits up to timeout for queued events to reach the server, and reports
// whether the queue drained. Worth calling before a process exits on purpose:
// the SDK sends in the background, so an immediate exit loses the last event.
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// Hub returns the hub bound to ctx, or a clone of the process-wide one when
// ctx carries none. Middleware that needs a request-scoped scope uses this;
// everything else should use the Context helpers above.
//
// The fallback clones rather than handing back the process-wide hub, because
// the reason to reach for a hub instead of CaptureExceptionContext is to set
// scope on it, and setting scope on the shared hub would leak one request's
// data onto every later event in the process.
func Hub(ctx context.Context) *sentry.Hub {
	if ctx != nil {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			return hub
		}
	}
	return sentry.CurrentHub().Clone()
}

// NewContext returns ctx carrying its own hub, so scope set on one request does
// not leak into another.
func NewContext(ctx context.Context) context.Context {
	return sentry.SetHubOnContext(ctx, sentry.CurrentHub().Clone())
}

func hubFrom(ctx context.Context) *sentry.Hub {
	if ctx != nil {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			return hub
		}
	}
	return sentry.CurrentHub()
}

// capture applies the options to a throwaway scope so they touch only this
// event, then hands the hub to send.
func capture(hub *sentry.Hub, opts []Option, send func(*sentry.Hub)) {
	if len(opts) == 0 {
		send(hub)
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		for _, opt := range opts {
			opt(scope)
		}
		send(hub)
	})
}

func summarize(event *sentry.Event) string {
	if event == nil {
		return "nil event"
	}

	parts := []string{}
	if event.EventID != "" {
		parts = append(parts, "event_id="+string(event.EventID))
	}
	if event.Message != "" {
		parts = append(parts, "message="+event.Message)
	}
	if len(event.Exception) > 0 {
		ex := event.Exception[0]
		exMsg := strings.TrimSpace(strings.TrimSpace(ex.Type + ": " + ex.Value))
		if exMsg != "" && exMsg != ":" {
			parts = append(parts, "exception="+exMsg)
		}
	}
	if len(parts) == 0 {
		return "captured event with no message"
	}

	return strings.Join(parts, " | ")
}
