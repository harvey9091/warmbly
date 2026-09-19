import { useQuery } from "@tanstack/react-query";
import getCampaignSendPlan from "@/lib/api/client/app/campaigns/getCampaignSendPlan";

// Today's sending plan. Realtime invalidation covers the sends and the
// settings; the clock is the one input no event announces (a window opening,
// a mailbox's hours, spacing running down), so the plan also re-reads itself
// once a minute while the page is open.
export default function useCampaignSendPlan(id: string) {
    return useQuery({
        queryKey: ["campaigns", id, "send-plan"],
        queryFn: () => getCampaignSendPlan(id),
        enabled: !!id,
        refetchInterval: 60_000,
        refetchOnWindowFocus: true,
    });
}
