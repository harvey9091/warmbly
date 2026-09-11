import { useMutation, useQueryClient } from "@tanstack/react-query";
import moveFolder from "@/lib/api/client/app/unibox/moveFolder";

export default function useMoveFolder() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: { ids: string[]; folder: "trash" | "archive" | "inbox" }) => moveFolder(data),
        onSuccess: () => {
            queryClient.invalidateQueries({
                queryKey: ["unibox"],
            })
        }
    })
}
