import Request from "../../Request";
import type SendPlan from "@/lib/api/models/app/campaigns/SendPlan";

export default async function getCampaignSendPlan(id: string): Promise<SendPlan> {
    return Request<SendPlan>({
        method: "GET",
        url: `/campaigns/${id}/send-plan`,
        authorization: true,
    });
}
