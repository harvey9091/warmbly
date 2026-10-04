import { useMutation, useQueryClient } from "@tanstack/react-query";
import updateSync from "@/lib/api/client/app/emails/updateSync";
import type EmailSync from "@/lib/api/models/app/emails/SyncState";

export default function useUpdateSyncSkipFolders(id: string) {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (skipFolders: string[]) => updateSync(id, skipFolders),
        onSuccess: (data) => {
            queryClient.setQueryData<EmailSync>(["emails", id, "sync"], (prev) =>
                prev ? { ...prev, skip_folders: data.skip_folders, policy: { ...prev.policy, skip_folders: data.skip_folders } } : prev,
            );
            // Mail already stored from a newly skipped folder is dropped on
            // the server, so every inbox list is stale.
            queryClient.invalidateQueries({ queryKey: ["unibox"] });
        },
    });
}
