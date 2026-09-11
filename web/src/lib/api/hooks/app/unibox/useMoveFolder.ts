import { useMutation, useQueryClient } from "@tanstack/react-query";
import moveFolder, { type FilableFolder } from "@/lib/api/client/app/unibox/moveFolder";

export default function useMoveFolder() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: { ids: string[]; folder: FilableFolder }) => moveFolder(data),
        // A move changes which scopes the thread belongs to and every folder's
        // counts, so the whole unibox tree is re-read rather than patched.
        onSettled: () => {
            queryClient.invalidateQueries({ queryKey: ["unibox"] })
        }
    })
}
