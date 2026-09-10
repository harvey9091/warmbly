//! Error reporting for the tracking service.
//!
//! Every failure path in this crate goes through `report_issue` or
//! `report_error`, so wiring a backend means wiring it here and nowhere else.
//!
//! Two backends exist and either, both or neither can be on: PostHog
//! (`POSTHOG_KEY`), which is the default, and Sentry (`SENTRY_DSN`), kept for
//! an operator who already runs it. Reporting is optional, like it is in every
//! other Warmbly service: with neither configured no SDK is initialised, no
//! host is contacted, and the failure is only written to the log.

use crate::posthog;
use sentry::ClientInitGuard;
use std::time::Duration;
use tracing::{error, info};

/// What one process knows about itself and its backends at boot.
pub struct Settings<'a> {
    pub env: &'a str,
    pub release: &'a str,
    pub sentry_dsn: &'a str,
    pub posthog_key: &'a str,
    pub posthog_host: &'a str,
}

/// How long a fatal path waits for its last event.
const FLUSH_TIMEOUT: Duration = Duration::from_secs(2);

/// Held for the lifetime of the process. Dropping it flushes queued events, so
/// `main` must keep it alive rather than binding it to `_`.
pub struct Guard(#[allow(dead_code)] Option<ClientInitGuard>);

/// Initialises reporting. No key and no DSN means local logging only.
pub fn init(settings: Settings<'_>) -> Guard {
    let Settings {
        env,
        release,
        sentry_dsn,
        posthog_key,
        posthog_host,
    } = settings;

    // Before sentry::init, so Sentry's own panic hook chains to this one rather
    // than replacing it.
    let posthog_enabled = posthog::init(posthog_key, posthog_host, env, release);
    if posthog_enabled {
        install_panic_hook();
    }

    let dsn = sentry_dsn.trim();
    let guard = if dsn.is_empty() {
        None
    } else {
        // A malformed DSN must not stop the service serving pixels: the pixel
        // is the product, error reporting is not.
        match dsn.parse::<sentry::types::Dsn>() {
            Ok(parsed) => {
                // ClientOptions is #[non_exhaustive], so it is built from the
                // default rather than as a struct literal.
                let mut options = sentry::ClientOptions::default();
                options.dsn = Some(parsed);
                options.environment = Some(env.to_owned().into());
                options.release = Some(release.to_owned().into());
                options.server_name = Some("tracking".into());
                // The `panic` feature installs the hook, so a panic in a
                // request handler reaches the same project as the errors
                // reported here.
                options.attach_stacktrace = true;
                Some(sentry::init(options))
            }
            Err(e) => {
                error!("Invalid SENTRY_DSN, Sentry reporting disabled: {e}");
                None
            }
        }
    };

    match (posthog_enabled, guard.is_some()) {
        (false, false) => info!("Issue reporting initialized (env={env}, local issue logging enabled)"),
        (posthog, sentry) => info!(
            "Issue reporting initialized (env={env}, release={release}, posthog={posthog}, sentry={sentry})"
        ),
    }
    Guard(guard)
}

/// Sends anything still queued, up to a couple of seconds.
///
/// `std::process::exit` skips destructors, so the guard's own flush on drop
/// never runs on a fatal path. Without this, the error a fatal path just
/// reported is exactly the one that never arrives.
pub fn flush() {
    posthog::flush(FLUSH_TIMEOUT);
    if let Some(client) = sentry::Hub::current().client() {
        client.flush(Some(FLUSH_TIMEOUT));
    }
}

pub fn report_issue(context: &str, details: &str) {
    error!("[issue-local][tracking] {context}: {details}");
    posthog::capture(context, details, true);
    sentry::capture_message(&format!("{context}: {details}"), sentry::Level::Error);
}

pub fn report_error(context: &str, err: &(dyn std::error::Error + Send + Sync)) {
    report_issue(context, &err.to_string());
}

/// Reports a panic to PostHog and then lets the previously installed hook run,
/// which is what keeps the default backtrace, and Sentry's hook when it is
/// installed after this one.
fn install_panic_hook() {
    let previous = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let message = info
            .payload()
            .downcast_ref::<&str>()
            .map(|s| (*s).to_string())
            .or_else(|| info.payload().downcast_ref::<String>().cloned())
            .unwrap_or_else(|| "panic".to_string());
        let value = match info.location() {
            Some(location) => format!("{message} at {}:{}", location.file(), location.line()),
            None => message,
        };
        posthog::capture("panic", &value, false);
        previous(info);
    }));
}
