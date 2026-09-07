import { useMutation, useQueryClient } from "@tanstack/react-query";
import startCampaign, { type StartCampaignOptions } from "@/lib/api/client/app/campaigns/startCampaign";
import { capture } from "@/lib/productAnalytics";

export default function useStartCampaign() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (arg: string | { id: string; options?: StartCampaignOptions }) =>
            typeof arg === "string" ? startCampaign(arg) : startCampaign(arg.id, arg.options),
        onSuccess: () => {
            queryClient.invalidateQueries({
                queryKey: ["campaigns"],
            })
            // Every page that starts a campaign goes through this hook, so the
            // event is counted once here rather than at each button.
            capture("campaign_launched");
        }
    })
}
