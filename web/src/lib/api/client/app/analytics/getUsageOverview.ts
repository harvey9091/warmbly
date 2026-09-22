import type UsageOverview from "@/lib/api/models/app/analytics/UsageOverview";
import Request from "../../Request";

export default async function getUsageOverview(period: "day" | "week" | "month" = "day"): Promise<UsageOverview> {
    return await Request<UsageOverview>({
        method: "GET",
        url: `/analytics/usage?period=${period}`,
        authorization: true,
    })
}
