// The dashboard's one PostHog client.
//
// Product analytics and error tracking are the same project and the same key,
// so they are the same SDK instance: two `posthog.init` calls would be two
// visitors, two pageview streams and two sets of global error handlers.
// `lib/productAnalytics` owns what we measure and `lib/observability` owns what
// we report; this file owns the client both of them borrow.
//
// Three rules decide the configuration below.
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
// why `person_profiles: 'never'` is set and why no analytics event property
// anywhere carries a user id, an organization id or an email.
//
// Exceptions are the one exception. An error nobody can trace to an account is
// an error nobody can answer a support message about, so `$exception` events,
// and only those, carry the workspace and user they happened to. That is a
// property on one event type, not an identity: no profile is created, nothing
// is stored in the browser, and analytics stays anonymous.
//
// Session replay stays off deliberately: it would record mailbox and contact
// screens. Exception steps are the alternative, and they are why an issue shows
// the route the user was on and the request that failed just before it.
import type { CaptureResult, PostHog, Properties } from "posthog-js";
import { maskIds } from "./maskIds";
import { POSTHOG_ERROR_TRACKING, POSTHOG_HOST, POSTHOG_KEY, POSTHOG_UI_HOST, SENTRY_ENVIRONMENT, SENTRY_RELEASE } from "./information";

let client: PostHog | null = null;
let loading: Promise<PostHog | null> | null = null;

// identity is attached to exceptions only. Held here rather than registered as
// a super property so it can never reach an analytics event.
let identity: Record<string, string> | null = null;

// loadPostHog resolves the initialised client, or null when no key is
// configured or the chunk could not be fetched. It initialises on the first
// call and every later caller gets the same instance.
export function loadPostHog(): Promise<PostHog | null> {
    if (!POSTHOG_KEY) return Promise.resolve(null);
    if (loading) return loading;

    loading = import("posthog-js")
        .then(({ posthog }) => {
            posthog.init(POSTHOG_KEY, {
                api_host: POSTHOG_HOST,
                ui_host: POSTHOG_UI_HOST,
                cookieless_mode: "always",
                person_profiles: "never",
                // The dashboard is a private tool behind a login. Autocapturing
                // every click would ship contact names and subject lines in
                // element text; the named events in productAnalytics are
                // deliberate instead.
                autocapture: false,
                // The dashboard is a single-page app, so page loads happen
                // once and every navigation after that is a history change.
                // Plain `true` would report one pageview per session.
                capture_pageview: "history_change",
                disable_session_recording: true,
                respect_dnt: true,
                // Error tracking. Console errors stay off: they are mostly
                // third-party noise and they would carry logged values.
                capture_exceptions: POSTHOG_ERROR_TRACKING
                    ? {
                          capture_unhandled_errors: true,
                          capture_unhandled_rejections: true,
                          capture_console_errors: false,
                      }
                    : false,
                before_send: decorate,
            });
            // Registered rather than passed per call so an autocaptured
            // exception carries them too, and so dashboard events are
            // separable from the admin panel's in a shared project.
            posthog.register(SENTRY_RELEASE
                ? { service: "dashboard", environment: SENTRY_ENVIRONMENT, release: SENTRY_RELEASE }
                : { service: "dashboard", environment: SENTRY_ENVIRONMENT });
            client = posthog;
            return posthog;
        })
        .catch(() => {
            // Blocked or failed: reporting is never a reason the dashboard breaks.
            return null;
        });

    return loading;
}

// postHogClient is the loaded client, or null while it is still in flight. Use
// it where dropping the call is better than waiting for one.
export function postHogClient(): PostHog | null {
    return client;
}

// setPostHogIdentity names the workspace and user that later exceptions belong
// to. Null on sign-out, so a shared machine's next session is not attributed to
// whoever used it last.
export function setPostHogIdentity(next: { organizationId?: string | null; userId?: string | null } | null): void {
    if (!next) {
        identity = null;
        return;
    }
    const resolved: Record<string, string> = {};
    if (next.organizationId) resolved.organization_id = next.organizationId;
    if (next.userId) resolved.user_id = next.userId;
    identity = Object.keys(resolved).length > 0 ? resolved : null;
}

// notePostHogStep records one step on the trail attached to the next exception.
// PostHog buffers them itself, oldest evicted past its byte budget.
//
// Steps recorded before the SDK loaded are held by lib/observability, which
// replays them the moment this client settles, so nothing buffers twice.
export function notePostHogStep(message: string, properties?: Properties): void {
    client?.addExceptionStep(message, properties);
}

// decorate is the last thing to touch an event before it is sent.
function decorate(event: CaptureResult | null): CaptureResult | null {
    const masked = maskIdsInURLs(event);
    if (!masked?.properties || masked.event !== "$exception") return masked;
    if (identity) Object.assign(masked.properties, identity);
    return masked;
}

// A pageview or an error event would otherwise ship the record id in the path.
function maskIdsInURLs(event: CaptureResult | null): CaptureResult | null {
    if (!event?.properties) return event;
    for (const key of ["$current_url", "$pathname", "$referrer"] as const) {
        const value = event.properties[key];
        if (typeof value === "string") {
            event.properties[key] = maskIds(value);
        }
    }
    return event;
}
