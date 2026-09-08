import type { LeadSyncSource } from "@/lib/api/models/app/leadsync/LeadSync";
import Request from "../../Request";

// Lists this org's saved sync sources, optionally filtered to one campaign or
// one segment (powers the global Contacts > Sync sources area, the per-campaign
// "Connect a Google Sheet" list and a segment's own sources).
export default async function listSources(
    campaignId?: string,
    segmentId?: string,
): Promise<{ data: LeadSyncSource[] }> {
    const params: Record<string, string> = {};
    if (campaignId) params.campaign_id = campaignId;
    if (segmentId) params.segment_id = segmentId;
    return await Request<{ data: LeadSyncSource[] }>({
        method: "GET",
        url: "/lead-sync/sources",
        params: Object.keys(params).length > 0 ? params : undefined,
        authorization: true,
    });
}
