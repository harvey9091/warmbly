// Address verification: who checks this workspace's contacts, and the member
// actions on a selection (re-verify, mark deliverable / undeliverable).

import Request from "../../Request";
import type ContactSelection from "@/lib/api/models/app/contacts/ContactSelection";
import type { ContactVerificationCounts } from "@/lib/api/models/app/contacts/SearchContactsResult";

export interface VerificationOverview {
    // "builtin" (the in-house check) or "millionverifier".
    provider: "builtin" | "millionverifier" | string;
    connection_id?: string;
    credits?: number;
    // Set when a provider is connected but unusable (bad key, no credits).
    provider_error?: string;
    // Whether the built-in check can reach mail servers from this instance.
    builtin_ready: boolean;
    counts: ContactVerificationCounts;
}

export type VerificationAction = "verify" | "mark_deliverable" | "mark_undeliverable";

export interface VerificationRequest extends Partial<ContactSelection> {
    // Every lead of this campaign that verification refused.
    campaign_id?: string;
    action: VerificationAction;
}

export interface VerificationResponse {
    affected: number;
    action: VerificationAction;
    queued: boolean;
}

export async function getContactVerification(): Promise<VerificationOverview> {
    return await Request<VerificationOverview>({
        method: "GET",
        url: "/contacts/verification",
        authorization: true,
    });
}

export async function requestContactVerification(req: VerificationRequest): Promise<VerificationResponse> {
    return await Request<VerificationResponse>({
        method: "POST",
        url: "/contacts/verification",
        data: req,
        authorization: true,
    });
}
