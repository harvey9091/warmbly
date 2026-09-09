import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
    listEmailImages,
    uploadEmailImage,
    deleteEmailImage,
} from "@/lib/api/client/app/campaigns/emailImages";

// The audit spine invalidates ['email-images'] on every teammate's upload or
// removal, so the library is live without a refetch interval.
const key = ["email-images"];

export function useEmailImages(enabled = true) {
    return useInfiniteQuery({
        queryKey: key,
        queryFn: ({ pageParam }) => listEmailImages(pageParam),
        initialPageParam: undefined as string | undefined,
        getNextPageParam: (last) => last.pagination.next_cursor ?? undefined,
        enabled,
    });
}

export function useUploadEmailImage() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (file: File) => uploadEmailImage(file),
        onSuccess: () => qc.invalidateQueries({ queryKey: key }),
    });
}

export function useDeleteEmailImage() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => deleteEmailImage(id),
        onSuccess: () => qc.invalidateQueries({ queryKey: key }),
    });
}
