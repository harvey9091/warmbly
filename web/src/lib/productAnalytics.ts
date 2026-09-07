// Cookieless product analytics.
//
// Two rules decide everything in this file.
//
// It is hosted-only. The dashboard image is the same for the hosted service and
// for a self-host, so the key comes from the container-injected runtime config
// and an unset key means the SDK chunk is never fetched and no PostHog host is
// ever contacted. A self-host therefore ships this code path and never runs it.
//
// It is cookieless, so there is no banner. `cookieless_mode: 'always'` stores
// nothing in the browser: no cookie, no localStorage, no sessionStorage. The
// visitor is derived server-side from a daily-rotated salt plus IP, root domain
// and user agent, and the salt is deleted at the end of the day, so there is no
// identifier to consent to. That only holds if we never call identify, which is
// why `person_profiles: 'never'` is set and why no event property below ever
// carries a user id, an organization id or an email.
//
// Session replay stays off deliberately: it would record mailbox and contact
// screens.
import type { CaptureResult, PostHog } from "posthog-js";
import { POSTHOG_HOST, POSTHOG_KEY } from "./information";

let client: PostHog | null = null;
let loading: Promise<void> | null = null;

// initProductAnalytics loads and configures the SDK, once, and only when a key
// is configured. Loaded as its own chunk so an install with no key pays neither
// the bytes nor a request.
export function initProductAnalytics(): void {
    if (!POSTHOG_KEY || loading) return;

    loading = import("posthog-js")
        .then(({ posthog }) => {
            posthog.init(POSTHOG_KEY, {
                api_host: POSTHOG_HOST,
                cookieless_mode: "always",
                person_profiles: "never",
                // The dashboard is a private tool behind a login. Autocapturing
                // every click would ship contact names and subject lines in
                // element text; the named events below are deliberate instead.
                autocapture: false,
                // The dashboard is a single-page app, so page loads happen
                // once and every navigation after that is a history change.
                // Plain `true` would report one pageview per session.
                capture_pageview: "history_change",
                disable_session_recording: true,
                respect_dnt: true,
                before_send: maskIdsInURLs,
            });
            client = posthog;
        })
        .catch(() => {
            // Blocked or failed: analytics is never a reason the dashboard breaks.
        });
}

// Dashboard paths carry record ids (/app/campaigns/<uuid>), and a pageview
// would otherwise ship them as $current_url. They are not personal data, but
// they are identifiers, and the whole point of cookieless mode is that no such
// value exists to join on. Every uuid-shaped segment becomes ":id", which is
// also what makes the pageview report readable: one row per screen instead of
// one row per record.
const UUID_SEGMENT = /\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?=\/|$)/gi;

function maskIdsInURLs(event: CaptureResult | null): CaptureResult | null {
    if (!event?.properties) return event;
    for (const key of ["$current_url", "$pathname", "$referrer"] as const) {
        const value = event.properties[key];
        if (typeof value === "string") {
            event.properties[key] = value.replace(UUID_SEGMENT, "/:id");
        }
    }
    return event;
}

// Event is the closed set of product events the dashboard reports. Keeping it a
// union rather than a string means a typo is a build error and the list stays
// readable as the answer to "what do we actually measure".
export type Event =
    | "mailbox_connected"
    | "campaign_launched";

// capture reports one product event. A no-op when analytics is off.
//
// Properties must stay non-identifying: a provider name or a step count is
// fine, an org id or an email address is not.
export function capture(event: Event, properties?: Record<string, string | number | boolean>): void {
    if (!POSTHOG_KEY) return;
    // The SDK may still be in flight on a fast first action; dropping the event
    // is better than queueing one that arrives without its session.
    client?.capture(event, properties);
}
