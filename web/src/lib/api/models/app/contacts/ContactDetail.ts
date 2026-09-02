import type Contact from "./Contact";

export interface ContactEngagement {
    total_sent: number;
    total_opened: number;
    total_clicked: number;
    total_replied: number;
    total_bounced: number;
    total_complained: number;

    last_sent_at?: string | null;
    last_opened_at?: string | null;
    last_clicked_at?: string | null;
    last_replied_at?: string | null;
    last_bounced_at?: string | null;
}

export interface ContactSuppression {
    reason: string;
    source: "bounce" | "complaint" | "unsubscribe" | string;
    expires_at?: string | null;
    created_at: string;
}

// Where a contact first came from. Mirrors the backend CHECK; "unknown" is
// what rows created before attribution existed carry.
export type ContactSource =
    | "unknown"
    | "manual"
    | "campaign"
    | "import"
    | "sheet_sync"
    | "api"
    | "ai_assistant"
    | "form";

// One observed fact about the mailbox. Silence is never recorded: a contact
// who does not open or reply has said nothing about their address.
export type VerificationEvidenceKind =
    | "delivered"
    | "opened"
    | "clicked"
    | "replied"
    | "auto_replied"
    | "bounced_recipient"
    | "bounced_other";

export interface ContactVerificationEvidence {
    kind: VerificationEvidenceKind;
    detail?: string;
    observed_at: string;
}

export interface ContactVerificationDetail {
    status: "valid" | "risky" | "invalid" | "unknown";
    confidence: number;
    reasons: string[];
    // True when real mail, not a check, decided the status.
    decisive: boolean;
    evidence: ContactVerificationEvidence[];
}

export default interface ContactDetail extends Contact {
    engagement: ContactEngagement;
    suppression?: ContactSuppression | null;
    verification?: ContactVerificationDetail | null;

    // First-touch attribution; never changes after creation.
    source: ContactSource;
    source_detail: string;
    first_seen_at: string;
}
