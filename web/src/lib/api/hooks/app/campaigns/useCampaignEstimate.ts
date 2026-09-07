import { useQuery } from "@tanstack/react-query";
import estimateCampaign, { type CampaignEstimateInput } from "@/lib/api/client/app/campaigns/estimateCampaign";

// Audience-versus-pool projection for the campaign wizard. Keyed on the whole
// input so every edit (segment, tag, limit, days, start) re-asks; disabled
// until at least one segment is chosen because the answer is zero before that.
export default function useCampaignEstimate(input: CampaignEstimateInput, enabled = true) {
    return useQuery({
        queryKey: ["campaigns", "estimate", input],
        queryFn: () => estimateCampaign(input),
        enabled: enabled && input.segment_ids.length > 0,
        staleTime: 30_000,
        placeholderData: (prev) => prev,
    });
}
