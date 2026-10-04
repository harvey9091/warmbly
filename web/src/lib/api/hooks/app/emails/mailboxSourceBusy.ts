// Mutations an import dialog must not be closed under: every one is keyed below this root.
import { useIsMutating } from "@tanstack/react-query";

export const MAILBOX_SOURCE_MUTATION = ["mailbox-source"] as const;

export function sourceMutationKey(name: string) {
    return [...MAILBOX_SOURCE_MUTATION, name];
}

/** True while an import, a vendor key or an admin grant request is in flight. */
export function useMailboxSourceBusy(): boolean {
    return useIsMutating({ mutationKey: MAILBOX_SOURCE_MUTATION }) > 0;
}
