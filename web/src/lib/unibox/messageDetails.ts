// Pure formatting for the message details panel and the header's recipient line.

import { bareEmail, nameFromAddr } from "@/lib/helper/emailAddress";
import type UniboxEmail from "@/lib/api/models/app/unibox/UniboxEmail";
import type { UniboxEmailDetail } from "@/lib/api/models/app/unibox/UniboxEmail";
import type { UniboxFolder } from "@/lib/api/models/app/unibox/UniboxSearch";

export interface AddressParts {
    /** Display name; empty when the header carried only an address. */
    name: string;
    address: string;
}

// A bare address carries both roles, so `name` stays empty rather than repeating it.
export function addressParts(raw: string): AddressParts {
    const address = bareEmail(raw);
    const name = nameFromAddr(raw);
    return { name: name === address ? "" : name, address };
}

// The To list: the full fetch when it has landed, else what the thread row carried.
export function recipientsOf(email: UniboxEmail, detail?: UniboxEmailDetail): string[] {
    if (detail?.to?.length) return detail.to;
    if (email.recipients?.length) return email.recipients;
    return email.to ? [email.to] : [];
}

// "Alice, Bob and 2 more"; the address stands in for a missing name.
export function summarizeAddresses(list: string[], max = 2): string {
    const names = list
        .map((a) => addressParts(a))
        .map((p) => p.name || p.address)
        .filter(Boolean);
    if (names.length === 0) return "";
    if (names.length <= max) return names.join(", ");
    return `${names.slice(0, max).join(", ")} and ${names.length - max} more`;
}

// Bare-address equality, so a Reply-To that repeats the sender is not a second row.
export function sameAddress(a: string, b: string): boolean {
    return addressParts(a).address.toLowerCase() === addressParts(b).address.toLowerCase();
}

// Keyed on the union so a new canonical folder fails typecheck here.
const FOLDER_LABELS: Record<UniboxFolder, string> = {
    inbox: "Inbox",
    sent: "Sent",
    drafts: "Drafts",
    archive: "Archive",
    spam: "Spam",
    trash: "Trash",
};

export function folderLabel(folder?: string): string {
    return (folder && (FOLDER_LABELS as Record<string, string>)[folder]) || "";
}

// Locale and zone of the reader, zone named, so a cross-border time is unambiguous.
export function formatExactTime(d: Date, locale?: string, timeZone?: string): string {
    return d.toLocaleString(locale, {
        weekday: "short",
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        timeZoneName: "short",
        timeZone,
    });
}

const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 31_536_000_000],
    ["month", 2_592_000_000],
    ["week", 604_800_000],
    ["day", 86_400_000],
    ["hour", 3_600_000],
    ["minute", 60_000],
];

// Whole units of the largest that fits, in words: "3 days ago", "in 2 hours".
export function relativeTime(d: Date, now = Date.now(), locale?: string): string {
    const diff = d.getTime() - now;
    const abs = Math.abs(diff);
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
    for (const [unit, ms] of RELATIVE_UNITS) {
        if (abs >= ms) return rtf.format(Math.trunc(diff / ms), unit);
    }
    return "just now";
}

// A received row is worth showing only when it disagrees with sent beyond clock skew.
export function receivedDiffers(sent: Date, received: Date, toleranceMs = 60_000): boolean {
    if (Number.isNaN(sent.getTime()) || Number.isNaN(received.getTime())) return false;
    return Math.abs(received.getTime() - sent.getTime()) > toleranceMs;
}
