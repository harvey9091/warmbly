import { useQuery } from "@tanstack/react-query";
import listSources from "@/lib/api/client/app/leadsync/listSources";

// Lists saved sync sources, optionally filtered to one campaign or one segment.
// Used by the global Contacts > Sync sources area, the per-campaign list and a
// segment's own sources.
export default function useLeadSyncSources(campaignId?: string, segmentId?: string) {
    return useQuery({
        queryKey: ["lead-sync", "sources", campaignId ?? null, segmentId ?? null],
        queryFn: () => listSources(campaignId, segmentId),
        staleTime: 10_000,
    });
}
