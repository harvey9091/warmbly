// Browser error reporting for the hosted form page.
//
// Public pages have to stay light, so unlike the dashboard this loads the SDK
// as its own chunk and only when the shell stamped a DSN: with none, which is
// every self-host and every install that has not configured one, nothing is
// fetched and nothing is sent. The cost is that an error thrown in the first
// few milliseconds is missed, which is the right trade on a page whose whole
// job is to render one form for a stranger.

function meta(name: string): string {
    return document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)?.content?.trim() ?? "";
}

export function initErrorReporting(): void {
    const dsn = meta("wf-sentry-dsn");
    if (!dsn) return;

    void import("@sentry/browser").then((Sentry) => {
        Sentry.init({
            dsn,
            release: meta("wf-release") || undefined,
            environment: meta("wf-environment") || undefined,
            // Named so form-page errors are separable from the dashboard's in
            // a shared project, the same way the Go services set ServerName.
            initialScope: { tags: { service: "forms" } },
            // A form page carries a stranger's answers. Default PII (their IP,
            // their headers) is not ours to collect, and the dashboard's
            // reasons for sending it do not apply here.
            sendDefaultPii: false,
        });
    }).catch(() => {
        // A blocked or failed SDK load must never stop the form rendering.
    });
}
