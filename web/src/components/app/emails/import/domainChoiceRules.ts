// The per-domain import choices as data: defaults, validation and the options they become.
import type { DomainRedirect, TrackingSuggestion } from "@/lib/api/models/app/emails/SendingDomain";
import type { MailboxImportOptions } from "@/lib/api/models/app/emails/MailboxImport";
import { noDnsControl, redirectTargetProblem } from "@/components/app/emails/domains/rules";

/** What the import knows about one domain. */
export interface DomainInfo {
    domain: string;
    /** Detected host; a shared consumer host offers no choices. */
    mail_host?: string;
    /** Absent when tracking is off on the instance. */
    tracking?: TrackingSuggestion | null;
    trackingLoading?: boolean;
    redirect?: DomainRedirect | null;
}

/** What the person changed; anything unset falls back to the default. */
export interface DomainPick {
    track?: boolean;
    redirect?: boolean;
    url?: string;
}

export type DomainPicks = Record<string, DomainPick>;

// Hosts whose domain belongs to the provider, so nobody here can add its DNS.
const SHARED_HOSTS = new Set(["gmail", "outlook", "yahoo", "aol", "icloud", "gmx", "yandex", "proton"]);

export function offersChoices(info: DomainInfo): boolean {
    return !noDnsControl(info.domain) && !SHARED_HOSTS.has(info.mail_host ?? "");
}

export function effectivePick(info: DomainInfo, pick: DomainPick | undefined) {
    const status = info.tracking?.status;
    return {
        track: !!info.tracking && (pick?.track ?? (status === "active" || status === "found")),
        redirect: pick?.redirect ?? false,
        url: pick?.url ?? info.redirect?.target_url ?? "",
    };
}

/** The picks as import options; keys absent when nothing is picked. */
export function domainOptions(infos: DomainInfo[], picks: DomainPicks): Pick<MailboxImportOptions, "tracking_domains" | "redirects"> {
    const tracking: Record<string, string> = {};
    const redirects: Record<string, string> = {};
    for (const info of infos) {
        if (!offersChoices(info)) continue;
        const e = effectivePick(info, picks[info.domain]);
        if (e.track && info.tracking?.host) tracking[info.domain] = info.tracking.host;
        if (e.redirect && e.url.trim()) redirects[info.domain] = e.url.trim();
    }
    return {
        ...(Object.keys(tracking).length > 0 ? { tracking_domains: tracking } : {}),
        ...(Object.keys(redirects).length > 0 ? { redirects } : {}),
    };
}

/** Why the picks cannot be sent, or null. */
export function domainChoiceIssue(infos: DomainInfo[], picks: DomainPicks): string | null {
    for (const info of infos) {
        if (!offersChoices(info)) continue;
        const e = effectivePick(info, picks[info.domain]);
        if (!e.redirect) continue;
        const problem = redirectTargetProblem(e.url, info.domain);
        if (problem) return `${info.domain}: ${problem} Or untick its redirect.`;
    }
    return null;
}

/** Domains left with DNS to add after the import: a redirect not live yet, or a tracking host still to point. */
export function dnsFollowUps(infos: DomainInfo[], picks: DomainPicks): number {
    let n = 0;
    for (const info of infos) {
        if (!offersChoices(info)) continue;
        const e = effectivePick(info, picks[info.domain]);
        const redirectDns = e.redirect && !(info.redirect?.verified && info.redirect.target_url === e.url.trim());
        const trackDns = e.track && info.tracking?.status === "suggested";
        if (redirectDns || trackDns) n++;
    }
    return n;
}
