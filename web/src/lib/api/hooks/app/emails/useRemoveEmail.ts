import removeEmail from "@/lib/api/client/app/emails/removeEmail";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import invalidateAfterMailboxRemoval from "./invalidateAfterMailboxRemoval";
import patchEmailLists from "./patchEmailLists";

export default function useRemoveEmail(id: string) {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: () => removeEmail(id),
        onSuccess: () => {
            // Drop the row first so the list reacts immediately, then refetch.
            patchEmailLists(queryClient, (rows) => rows.filter((c) => c.id !== id));
            void invalidateAfterMailboxRemoval(queryClient);
        },
    });
}
