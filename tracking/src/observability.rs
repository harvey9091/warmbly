//! Error reporting for the tracking service.
//!
//! Every failure path in this crate goes through `report_issue` or
//! `report_error`, so wiring Sentry means wiring it here and nowhere else.
//!
//! Reporting is optional, like it is in every other Warmbly service: with no
//! `SENTRY_DSN` the guard below is never created, the SDK starts no transport
//! thread and contacts no host, and the failure is only written to the log.

use sentry::ClientInitGuard;
use std::time::Duration;
use tracing::{error, info};

/// Held for the lifetime of the process. Dropping it flushes queued events, so
/// `main` must keep it alive rather than binding it to `_`.
pub struct Guard(#[allow(dead_code)] Option<ClientInitGuard>);

/// Initialises reporting. `dsn` empty means local logging only.
pub fn init(env: &str, dsn: Option<&str>, release: &str) -> Guard {
    let dsn = dsn.map(str::trim).filter(|d| !d.is_empty());

    let Some(dsn) = dsn else {
        info!("Issue reporting initialized (env={env}, local issue logging enabled)");
        return Guard(None);
    };

    // A malformed DSN must not stop the service serving pixels: the pixel is
    // the product, error reporting is not.
    let guard = match dsn.parse::<sentry::types::Dsn>() {
        Ok(parsed) => {
            // ClientOptions is #[non_exhaustive], so it is built from the
            // default rather than as a struct literal.
            let mut options = sentry::ClientOptions::default();
            options.dsn = Some(parsed);
            options.environment = Some(env.to_owned().into());
            options.release = Some(release.to_owned().into());
            options.server_name = Some("tracking".into());
            // The `panic` feature installs the hook, so a panic in a request
            // handler reaches the same project as the errors reported here.
            options.attach_stacktrace = true;
            Some(sentry::init(options))
        }
        Err(e) => {
            error!("Invalid SENTRY_DSN, error reporting disabled: {e}");
            None
        }
    };

    if guard.is_some() {
        info!("Issue reporting initialized (env={env}, release={release}, sentry enabled)");
    }
    Guard(guard)
}

/// Sends anything still queued, up to a couple of seconds.
///
/// `std::process::exit` skips destructors, so the guard's own flush on drop
/// never runs on a fatal path. Without this, the error a fatal path just
/// reported is exactly the one that never arrives.
pub fn flush() {
    if let Some(client) = sentry::Hub::current().client() {
        client.flush(Some(Duration::from_secs(2)));
    }
}

pub fn report_issue(context: &str, details: &str) {
    error!("[issue-local][tracking] {context}: {details}");
    sentry::capture_message(&format!("{context}: {details}"), sentry::Level::Error);
}

pub fn report_error(context: &str, err: &(dyn std::error::Error + Send + Sync)) {
    report_issue(context, &err.to_string());
}
