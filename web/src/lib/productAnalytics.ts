// Cookieless product analytics.
//
// The client itself lives in lib/posthog, which product analytics and error
// tracking share. This file is only the closed list of what we measure and the
// rule that keeps it non-identifying.
import { loadPostHog, postHogClient } from "./posthog";
import { POSTHOG_KEY } from "./information";

// initProductAnalytics loads and configures the SDK, once, and only when a key
// is configured. Loaded as its own chunk so an install with no key pays neither
// the bytes nor a request.
export function initProductAnalytics(): void {
    void loadPostHog();
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
    postHogClient()?.capture(event, properties);
}
