import type DirectMailAnalytics from "@/lib/api/models/app/analytics/DirectMailAnalytics";
import Request from "../../Request";

// Hand-written mail analytics. Bare object, same as the dashboard endpoint.
export default async function getDirectMail(period: string = "7d"): Promise<DirectMailAnalytics> {
    return await Request<DirectMailAnalytics>({
        method: "GET",
        url: `/analytics/direct?period=${encodeURIComponent(period)}`,
        authorization: true,
    })
}
