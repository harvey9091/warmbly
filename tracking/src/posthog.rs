//! PostHog error tracking for the tracking service.
//!
//! There is no PostHog crate in this build, and there does not need to be: an
//! `$exception` event is one JSON POST to the same capture endpoint the
//! backend's analytics already uses, and the payload shape is the one PostHog
//! documents for manual capture. That keeps the pixel's dependency list, and so
//! its image, unchanged.
//!
//! Sending happens on one dedicated thread with a blocking client rather than
//! spawned onto the request runtime, so a slow or unreachable host can never
//! delay a pixel, and so `flush` on a fatal path has something to wait on.

use serde_json::{json, Value};
use std::sync::mpsc::{sync_channel, Receiver, SyncSender};
use std::sync::OnceLock;
use std::time::Duration;
use tracing::warn;

/// PostHog Cloud US, where most of the customers are.
const DEFAULT_HOST: &str = "https://us.i.posthog.com";

/// PostHog's single-event capture endpoint.
const CAPTURE_PATH: &str = "/i/v0/e/";

/// One send. Long enough for a slow host, short enough that a stuck queue
/// drains rather than grows.
const SEND_TIMEOUT: Duration = Duration::from_secs(5);

/// Queue depth. A service reporting faster than this drops rather than grows a
/// backlog: the newest errors are worth no more than the pixel's memory.
const QUEUE_DEPTH: usize = 64;

static REPORTER: OnceLock<Reporter> = OnceLock::new();

enum Message {
    Event(Value),
    /// Acked once every event queued ahead of it has been sent.
    Drain(SyncSender<()>),
}

struct Reporter {
    tx: SyncSender<Message>,
    key: String,
    context: Value,
}

/// Starts the sender thread. Returns false when no key is configured, which is
/// the default and means nothing is ever sent.
pub fn init(key: &str, host: &str, env: &str, release: &str) -> bool {
    let key = key.trim();
    if key.is_empty() {
        return false;
    }

    let host = match host.trim().trim_end_matches('/') {
        "" => DEFAULT_HOST.to_string(),
        h => h.to_string(),
    };

    let (tx, rx) = sync_channel(QUEUE_DEPTH);
    std::thread::Builder::new()
        .name("posthog-errors".into())
        .spawn(move || send_loop(host, rx))
        .ok();

    REPORTER
        .set(Reporter {
            tx,
            key: key.to_string(),
            context: json!({
                "service": "tracking",
                "environment": env,
                "release": release,
            }),
        })
        .is_ok()
}

/// Queues one exception. Never blocks: a full queue drops the event.
pub fn capture(exception_type: &str, value: &str, handled: bool) {
    let Some(reporter) = REPORTER.get() else {
        return;
    };

    let mut properties = json!({
        "$exception_list": [{
            "type": exception_type,
            "value": value,
            "mechanism": { "handled": handled, "synthetic": false },
        }],
        "$exception_level": "error",
        // No person is created or updated by an exception: the distinct id
        // names a process, not somebody.
        "$process_person_profile": false,
        // A server's IP is the datacentre's, so geolocating it says nothing.
        "$geoip_disable": true,
    });
    if let (Some(props), Some(context)) = (properties.as_object_mut(), reporter.context.as_object())
    {
        for (k, v) in context {
            props.insert(k.clone(), v.clone());
        }
    }

    let event = json!({
        "api_key": reporter.key,
        "event": "$exception",
        "distinct_id": "warmbly-tracking",
        "properties": properties,
        "timestamp": chrono::Utc::now().to_rfc3339(),
    });

    let _ = reporter.tx.try_send(Message::Event(event));
}

/// Waits up to `timeout` for the queue to drain. `std::process::exit` skips
/// every destructor, so without this the error a fatal path just reported is
/// exactly the one that never arrives.
pub fn flush(timeout: Duration) {
    let Some(reporter) = REPORTER.get() else {
        return;
    };
    let (ack, done) = sync_channel(1);
    // try_send, not send: a full queue against a dead host would block the exit
    // path for as long as it takes to time out every event ahead of us, and a
    // queue that full is not going to drain inside `timeout` anyway.
    if reporter.tx.try_send(Message::Drain(ack)).is_ok() {
        let _ = done.recv_timeout(timeout);
    }
}

fn send_loop(host: String, rx: Receiver<Message>) {
    let client = match reqwest::blocking::Client::builder()
        .timeout(SEND_TIMEOUT)
        .build()
    {
        Ok(c) => c,
        Err(e) => {
            warn!("PostHog error tracking disabled, no HTTP client: {e}");
            return;
        }
    };
    let url = format!("{host}{CAPTURE_PATH}");
    // A wrong key fails on every event, so the failure is logged once rather
    // than once per error; logging none of them would make a misconfigured key
    // look exactly like a quiet week.
    let mut warned = false;

    while let Ok(message) = rx.recv() {
        match message {
            Message::Drain(ack) => {
                let _ = ack.try_send(());
            }
            Message::Event(event) => {
                let result = client.post(&url).json(&event).send();
                if warned {
                    continue;
                }
                match result {
                    Ok(response) if !response.status().is_success() => {
                        warn!(
                            "PostHog rejected an error report with {} (check POSTHOG_KEY)",
                            response.status()
                        );
                        warned = true;
                    }
                    Err(e) => {
                        warn!("cannot reach the PostHog host {url}: {e}");
                        warned = true;
                    }
                    _ => {}
                }
            }
        }
    }
}
