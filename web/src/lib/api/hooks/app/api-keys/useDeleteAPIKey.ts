import { useMutation, useQueryClient } from "@tanstack/react-query";
import deleteAPIKey from "@/lib/api/client/app/api-keys/deleteAPIKey";

export default function useDeleteAPIKey() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (id: string) => deleteAPIKey(id),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["api-keys"] });
        },
    });
}
