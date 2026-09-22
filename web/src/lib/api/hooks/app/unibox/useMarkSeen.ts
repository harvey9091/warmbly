import { useMutation, useQueryClient, type InfiniteData } from "@tanstack/react-query";
import markSeen from "@/lib/api/client/app/unibox/markSeen";
import type { UniboxListRow } from "@/lib/api/client/app/unibox/searchIncoming";
import type UniboxThread from "@/lib/api/models/app/unibox/UniboxThread";

interface SearchPage {
    data: UniboxListRow[];
    pagination: { has_more: boolean; next_cursor: string | null };
}

interface MarkSeenInput {
    ids?: string[];
    folder?: string;
    seen?: boolean;
    /**
     * Conversations to flip. The open list is patched in place, and the server
     * flips every message in each, which a list row could not name.
     */
    threadIds?: string[];
}

export default function useMarkSeen() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: ({ ids, folder, seen, threadIds }: MarkSeenInput) =>
            markSeen({ ids, threadIds, folder, seen }),
        // Reading a thread must not move the list the user is reading from.
        // Refetching every loaded page of ["unibox","search"] would re-order
        // rows around whatever arrived since, so the read/unread flip is
        // written straight into the cache instead; only the counters, which no
        // pointer is aimed at, are refetched.
        onMutate: async ({ ids, threadIds, seen = true, folder }) => {
            if (folder || (!threadIds?.length && !ids?.length)) return;
            // A refetch already in flight would land on top of the patch below
            // and put the row back to unread. Only refetches: cancelling a
            // first load would leave that list with no data and nothing queued
            // to fetch it again.
            await queryClient.cancelQueries({
                queryKey: ["unibox", "search"],
                predicate: (query) => query.state.data !== undefined,
            });
            const idSet = new Set(ids ?? []);
            // Rows with no thread id of their own are keyed by message id, the
            // same fallback the list and the server collapse on.
            const threadSet = new Set(threadIds ?? []);
            const matches = (row: { id: string; thread_id?: string }) =>
                threadSet.has(row.thread_id || row.id) || idSet.has(row.id);

            queryClient.setQueriesData<InfiniteData<SearchPage>>(
                { queryKey: ["unibox", "search"] },
                (old) =>
                    !old
                        ? old
                        : {
                              ...old,
                              pages: old.pages.map((page) => ({
                                  ...page,
                                  data: (page.data ?? []).map((row) =>
                                      matches(row) && row.has_unread === seen
                                          ? { ...row, has_unread: !seen, seen }
                                          : row,
                                  ),
                              })),
                          },
            );

            for (const threadId of threadIds ?? []) {
                queryClient.setQueriesData<UniboxThread>(
                    { queryKey: ["unibox", "thread", threadId] },
                    (old) =>
                        !old
                            ? old
                            : {
                                  ...old,
                                  data: (old.data ?? []).map((m) =>
                                      m.seen === seen ? m : { ...m, seen },
                                  ),
                              },
                );
            }
        },
        // A patch the server rejected has to come back off; re-reading the
        // lists is both the rollback and the resync.
        onError: () => {
            queryClient.invalidateQueries({ queryKey: ["unibox"] });
        },
        onSuccess: (_data, { folder }) => {
            // A folder sweep touches rows we have no ids for, so that one still
            // has to re-read the list.
            if (folder) queryClient.invalidateQueries({ queryKey: ["unibox"] });
            else {
                queryClient.invalidateQueries({ queryKey: ["unibox", "overview"] });
                queryClient.invalidateQueries({ queryKey: ["unibox", "unseen-count"] });
            }
        },
    });
}
