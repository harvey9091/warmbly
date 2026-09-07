// Where a signup came from, read from the signup URL's query string.
//
// Nothing is stored in the browser: the marketing site appends these to its
// "start free" links (see site/src/layouts/Layout.astro), the dashboard reads
// them off its own URL at signup, and the backend writes them once onto the new
// organization. No cookie, no localStorage, and nothing captured before someone
// actually signs up.
//
// This is first-party on purpose. Cookieless analytics answers "which page
// converts" for a day, because the identifying salt is rotated daily; this
// answers "which channel pays" for as long as the account exists, and it keeps
// working if the analytics provider is blocked, changed or dropped.

export interface Acquisition {
    landing_path?: string;
    referrer_host?: string;
    utm_source?: string;
    utm_medium?: string;
    utm_campaign?: string;
    utm_term?: string;
    utm_content?: string;
}

// The parameters carried across. The five UTM names are the conventional set
// every ad platform and email tool already emits, so nothing has to be taught
// a Warmbly-specific parameter.
export const UTM_PARAMS = ["utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"] as const;

// Values are clamped here as well as on the server, so a hand-edited link
// cannot make the signup request enormous.
const MAX_LENGTH = 255;

function clamp(value: string | null): string | undefined {
    if (!value) return undefined;
    const trimmed = value.trim().slice(0, MAX_LENGTH);
    return trimmed || undefined;
}

// readAcquisition reads the current URL. Returns an empty object for a direct
// visit, which is most of them, and the backend then stores nothing.
export function readAcquisition(search: string = window.location.search): Acquisition {
    const params = new URLSearchParams(search);
    const acquisition: Acquisition = {};

    for (const name of UTM_PARAMS) {
        const value = clamp(params.get(name));
        if (value) acquisition[name] = value;
    }

    // wb_lp is the landing path the marketing site was on when the visitor
    // clicked through. Only a path is accepted: a full URL would carry the
    // referring page's own query string, which is not ours to store.
    const landing = clamp(params.get("wb_lp"));
    if (landing && landing.startsWith("/")) acquisition.landing_path = landing;

    // The referrer is reduced to a bare host for the same reason.
    const referrer = clamp(params.get("wb_ref")) ?? hostOf(document.referrer);
    if (referrer) acquisition.referrer_host = referrer;

    return acquisition;
}

// isEmpty reports a direct visit, so the caller can omit the field entirely.
export function isEmpty(acquisition: Acquisition): boolean {
    return Object.keys(acquisition).length === 0;
}

function hostOf(url: string): string | undefined {
    if (!url) return undefined;
    try {
        const host = new URL(url).hostname.toLowerCase();
        // Our own origin is not a referral.
        return host === window.location.hostname ? undefined : host;
    } catch {
        return undefined;
    }
}
