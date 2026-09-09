// Infinite scroll over the inbox search endpoint.

import { keepPreviousData, useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import searchIncoming from "@/lib/api/client/app/unibox/searchIncoming";
import type { UniboxSearchParams } from "@/lib/api/models/app/unibox/UniboxSearch";

type Page = Awaited<ReturnType<typeof searchIncoming>>;

export default function useUniboxSearch(params: UniboxSearchParams, enabled = true) {
    const q = useInfiniteQuery<
        Page,
        Error,
        InfiniteData<Page, string | null>,
        ["unibox", "search", UniboxSearchParams],
        string | null
    >({
        queryKey: ["unibox", "search", params],
        queryFn: ({ pageParam }) =>
            searchIncoming({ ...params, cursor: pageParam ?? undefined, limit: 50 }),
        initialPageParam: null,
        getNextPageParam: (last) => (last.pagination.has_more ? last.pagination.next_cursor : undefined),
        staleTime: 30_000,
        // Held long enough that leaving the inbox and coming back restores every
        // page the user had loaded, which is what the remembered scroll offset
        // needs to land on (issue #396).
        gcTime: 30 * 60 * 1000,
        // Scope/filter switches change the query key; keep showing the
        // previous list while the new one loads instead of flashing the
        // whole pane to skeletons on every switch.
        placeholderData: keepPreviousData,
        enabled,
    });

    // Flatten + drop any null rows just in case the server returns a nil
    // slice (defensive — mirrors the contacts / campaigns hooks).
    const emails =
        q.data?.pages
            .flatMap((p) => p.data ?? [])
            .filter((e): e is NonNullable<typeof e> => e != null) ?? [];

    return { ...q, emails };
}
