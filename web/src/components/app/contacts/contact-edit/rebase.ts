// Rebasing the 360 panel's draft onto a record that changed under it.
//
// The panel is a long-lived form over one contact, and the contact keeps
// moving while it is open: lifting a suppression on the Overview tab
// re-subscribes them, and a teammate's edit arrives through the audit spine.
// Adopting the new value wholesale would throw away what the user is typing;
// ignoring it leaves the draft disagreeing with a record nobody edited, which
// is what made closing the panel ask to discard changes that were never made
// (issue #415).
//
// So: field by field, keep what the user changed and take the server's value
// for everything they did not touch.

import type MiniCampaign from "@/lib/api/models/app/campaigns/MiniCampaign";
import type { CustomField } from "./DetailsTab";

export function rebase<T>(
    local: T,
    prevServer: T,
    nextServer: T,
    same: (a: T, b: T) => boolean = Object.is,
): T {
    return same(local, prevServer) ? nextServer : local;
}

// Set equality on ids: order is not meaningful for campaigns or categories,
// and this is the comparison the panel's `dirty` check uses.
export function sameIDs(a: string[], b: string[]): boolean {
    if (a.length !== b.length) return false;
    const set = new Set(b);
    return a.every((id) => set.has(id));
}

export function idsOf(items: { id: string }[]): string[] {
    return items.map((i) => i.id);
}

export function sameCampaigns(a: MiniCampaign[], b: MiniCampaign[]): boolean {
    return sameIDs(idsOf(a), idsOf(b));
}

// The custom-field rows as the record they save as: unnamed rows dropped,
// names trimmed.
export function recordFromCF(fields: CustomField[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const f of fields) {
        if (!f.name.trim()) continue;
        out[f.name.trim()] = f.value;
    }
    return out;
}

export function fieldsOf(record: Record<string, string> | undefined): CustomField[] {
    return Object.entries(record ?? {}).map(([name, value]) => ({ name, value }));
}

export function sameFields(a: CustomField[], b: CustomField[]): boolean {
    return JSON.stringify(recordFromCF(a)) === JSON.stringify(recordFromCF(b));
}
