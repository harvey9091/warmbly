// /emails/domains: every domain the workspace sends from, with its custom
// tracking host and the optional redirect of its root to the company website.
import type TrackingDomain from "./TrackingDomain";

export type DomainAuthState = "passing" | "failing" | "unknown";

/** One custom tracking host the domain's mailboxes use. */
export interface TrackingDomainUse {
    host: string;
    verified: boolean;
    mailboxes: number;
}

export type DNSRecordPurpose = "ownership" | "root" | "www";

/** One record to add at the DNS provider, and whether it is in place. */
export interface DNSRecord {
    purpose: DNSRecordPurpose;
    type: string;
    name: string;
    value: string;
    ok: boolean;
    /** Improves the result but is not needed to verify. */
    optional?: boolean;
}

export interface DomainRedirect {
    id: string;
    domain: string;
    target_url: string;
    include_www: boolean;
    verified: boolean;
    verified_at?: Date | null;
    last_checked_at?: Date | null;
    last_error?: string;
    created_at: Date;
    records: DNSRecord[];
}

export interface SendingDomain {
    domain: string;
    mailboxes: number;
    mail_hosts: string[];
    auth_state: DomainAuthState;
    auth_spf: boolean;
    auth_dkim: boolean;
    auth_dmarc: boolean;
    tracking_domains: TrackingDomainUse[];
    redirect?: DomainRedirect | null;
    /** Inbox vendors this domain's mailboxes were imported from. */
    vendors?: string[];
    vendor_connection_ids?: string[];
    /** Set when a connected vendor account holds the domain. */
    vendor_domain?: VendorDomainLink | null;
}

/** A sending domain an inbox vendor account holds, and what its API can do for it. */
export interface VendorDomainLink {
    vendor: string;
    connection_id: string;
    /** Where the vendor forwards the bare domain today, if anywhere. */
    forwarding?: string;
    can_forward: boolean;
    /** An empty target removes the forwarding. */
    can_unforward: boolean;
    /** The vendor's staff apply a forwarding change by hand, later. */
    forwarding_reviewed: boolean;
    can_dns: boolean;
    dns_types: string[];
}

/**
 * The tracking host to offer a domain: "active" is already in use and verified,
 * "found" already points at this instance, "suggested" still needs its CNAME.
 */
export interface TrackingSuggestion {
    host: string;
    status: "active" | "found" | "suggested";
    cname_target: string;
}

export interface SetDomainTrackingResult {
    tracking: TrackingDomain;
    /** Mailboxes the host was put on. */
    mailboxes: number;
}

export interface SetDomainRedirectRequest {
    target_url: string;
    include_www?: boolean;
}
