import type InboxTagReview from "@/lib/api/models/app/inboxtag/InboxTagReview";
import Request from "../../Request";

export default async function getInboxTagReview(
    needsReviewOnly = false,
    limit = 50,
    cursor?: string,
): Promise<InboxTagReview> {
    const usp = new URLSearchParams({ limit: String(limit) });
    if (needsReviewOnly) usp.set("needs_review", "true");
    if (cursor) usp.set("cursor", cursor);
    return await Request<InboxTagReview>({
        method: "GET",
        url: `/analytics/inbox-tagging?${usp.toString()}`,
        authorization: true,
    })
}
