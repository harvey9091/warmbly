import { useInfiniteQuery } from "@tanstack/react-query";
import getInboxTagReview from "@/lib/api/client/app/inboxtag/getInboxTagReview";

export default function useInboxTagReview(needsReviewOnly = false) {
    return useInfiniteQuery({
        queryKey: ["inbox-tagging", "review", needsReviewOnly],
        queryFn: ({ pageParam }) => getInboxTagReview(needsReviewOnly, 50, pageParam),
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (lastPage) => lastPage.pagination.next_cursor ?? undefined,
    })
}
