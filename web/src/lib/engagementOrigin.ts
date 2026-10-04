// How an open or click says it was read, in words: the device ("iPhone",
// "Windows PC"), the mail client ("Apple Mail app", "Outlook on the web"),
// and why the device is sometimes unknowable. Every surface that shows an
// engagement origin (contact timeline, live campaign feed, recent activity,
// the contact overview) goes through here so they all say the same thing.

export interface OriginLike {
    client?: string;
    client_type?: string; // "app" | "webmail" | ""
    device_hidden?: boolean;
    device_type?: string; // "desktop" | "mobile" | "tablet" | "" | "unknown"
    os?: string;
    browser?: string;
    browser_version?: string;
    country_code?: string;
    region?: string;
    city?: string;
}

export type DeviceKind = "mobile" | "tablet" | "desktop" | "hidden" | "unknown";

export type EngagementKind = "open" | "click";

export function deviceKind(o: OriginLike): DeviceKind {
    if (o.device_hidden) return "hidden";
    switch (o.device_type) {
        case "mobile":
        case "tablet":
        case "desktop":
            return o.device_type;
        default:
            return "unknown";
    }
}

// "iPhone", "Android phone", "Mac", "Windows PC"; the bare type when the
// operating system is unknown; empty when nothing is known.
export function deviceLabel(o: OriginLike): string {
    const os = (o.os ?? "").toLowerCase();
    switch (deviceKind(o)) {
        case "hidden":
            return "Device hidden";
        case "mobile":
            if (os === "ios") return "iPhone";
            if (os === "android") return "Android phone";
            return "Mobile";
        case "tablet":
            if (os === "ios" || os === "ipados") return "iPad";
            if (os === "android") return "Android tablet";
            return "Tablet";
        case "desktop":
            if (os === "macos") return "Mac";
            if (os === "windows") return "Windows PC";
            if (os === "chromeos") return "Chromebook";
            if (os === "linux") return "Linux";
            return "Desktop";
        default:
            return "";
    }
}

// What the email was read in: "Apple Mail app", "Outlook on the web",
// "Gmail", "Webmail in Chrome". For a click it is where the link opened:
// the in-app browser's client when named, otherwise the browser.
export function readerLabel(o: OriginLike, kind: EngagementKind = "open"): string {
    const client = (o.client ?? "").trim();
    const browser = (o.browser ?? "").trim();
    if (kind === "click") return client || browser;
    if (client) {
        if (o.client_type === "app") return /\bapp$/i.test(client) ? client : `${client} app`;
        if (o.client_type === "webmail") return /\bweb(mail)?$/i.test(client) ? client : `${client} on the web`;
        return client;
    }
    if (o.client_type === "webmail") return browser ? `Webmail in ${browser}` : "Webmail";
    if (o.client_type === "app") return "Mail app";
    return browser;
}

// "iPhone · Apple Mail app", "Gmail · device hidden", "Windows PC · Chrome";
// empty when the event said nothing about itself.
export function originSummary(o: OriginLike, kind: EngagementKind = "open"): string {
    const reader = readerLabel(o, kind);
    if (o.device_hidden) return reader ? `${reader} · device hidden` : "Device hidden";
    return [deviceLabel(o), reader].filter(Boolean).join(" · ");
}

// Why the device is unknown, for a tooltip or a detail row. Empty when the
// device was not hidden by a proxy.
export function hiddenReason(o: OriginLike): string {
    if (!o.device_hidden) return "";
    switch (o.client) {
        case "Gmail":
            return "Gmail loads images through Google's servers, on the web and in the Gmail apps alike, so the reader's device and location are not visible.";
        case "Apple Mail":
            return "Apple Mail Privacy Protection loads images through Apple's relay, so the device is hidden and the location is only the rough region.";
        case "":
        case undefined:
            return "A mail provider's image proxy loaded this, so the reader's device and location are not visible.";
        default:
            return `${o.client} loads images through its own servers, so the reader's device and location are not visible.`;
    }
}

// "Berlin, DE" or "" when the network could not be placed.
export function originPlace(o: OriginLike): string {
    return [o.city, o.country_code].filter(Boolean).join(", ");
}
