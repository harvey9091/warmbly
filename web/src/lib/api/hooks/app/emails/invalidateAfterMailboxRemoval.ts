import type { QueryClient } from "@tanstack/react-query";

// Everything a removed mailbox leaves behind in the cache: its inbox mail and
// unread badge, the advice about it, the campaigns that sent through it and the
// analytics it fed. ["emails"] also covers the allowance counter, which a
// removal gives a slot back to.
export const MAILBOX_REMOVAL_KEYS = [["emails"], ["unibox"], ["advisor"], ["campaigns"], ["analytics"]] as const;

export default function invalidateAfterMailboxRemoval(queryClient: QueryClient) {
    return Promise.all(
        MAILBOX_REMOVAL_KEYS.map((queryKey) => queryClient.invalidateQueries({ queryKey: [...queryKey] })),
    );
}
