// Browser error reporting for the hosted form page.
//
// PostHog is the default backend and Sentry is still supported; the Go shell
// stamps whichever the operator configured, and with neither, which is every
// self-host and every install that has not configured one, nothing is fetched
// and nothing is sent.
//
// Public pages have to stay light, so unlike the dashboard each SDK loads as
// its own chunk. The cost is that an error thrown in the first few
// milliseconds is missed, which is the right trade on a page whose whole job is
// to render one form for a stranger.

function meta(name: string): string {
    return document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)?.content?.trim() ?? "";
}

export function initErrorReporting(): void {
    const release = meta("wf-release") || undefined;
    const environment = meta("wf-environment") || undefined;

    const posthogKey = meta("wf-posthog-key");
    if (posthogKey) {
        void import("posthog-js").then(({ posthog }) => {
            posthog.init(posthogKey, {
                api_host: meta("wf-posthog-host") || "https://us.i.posthog.com",
                // A form page carries a stranger's answers, so nothing is
                // stored in their browser and no profile is ever built: the
                // visitor is a daily-rotated hash that PostHog then deletes.
                cookieless_mode: "always",
                person_profiles: "never",
                autocapture: false,
                capture_pageview: false,
                disable_session_recording: true,
                respect_dnt: true,
                capture_exceptions: {
                    capture_unhandled_errors: true,
                    capture_unhandled_rejections: true,
                    capture_console_errors: false,
                },
            });
            // Named so form-page errors are separable from the dashboard's in a
            // shared project, the same way the Go services set a service
            // property.
            posthog.register(release
                ? { service: "forms", environment, release }
                : { service: "forms", environment });
        }).catch(() => {
            // A blocked or failed SDK load must never stop the form rendering.
        });
    }

    const dsn = meta("wf-sentry-dsn");
    if (dsn) {
        void import("@sentry/browser").then((Sentry) => {
            Sentry.init({
                dsn,
                release,
                environment,
                // Named so form-page errors are separable from the dashboard's
                // in a shared project, the same way the Go services set
                // ServerName.
                initialScope: { tags: { service: "forms" } },
                // A form page carries a stranger's answers. Default PII (their
                // IP, their headers) is not ours to collect, and the
                // dashboard's reasons for sending it do not apply here.
                sendDefaultPii: false,
            });
        }).catch(() => {
            // A blocked or failed SDK load must never stop the form rendering.
        });
    }
}
