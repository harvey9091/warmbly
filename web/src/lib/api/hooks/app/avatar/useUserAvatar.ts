import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import {
    deleteUserAvatar,
    uploadUserAvatar,
} from "@/lib/api/client/app/avatar/uploadUserAvatar";
import type User from "@/lib/api/models/auth/User";

// Patch the cached /auth/me user with the server's answer so the sidebar and
// profile page flip immediately, then invalidate so the refetch confirms it.
function applyUserAvatar(qc: QueryClient, avatarUrl: string | null) {
    qc.setQueryData<User>(["auth", "me"], (old) => (old ? { ...old, avatar_url: avatarUrl } : old));
    qc.invalidateQueries({ queryKey: ["auth", "me"] });
}

export function useUploadUserAvatar() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (blob: Blob) => uploadUserAvatar(blob),
        onSuccess: (res) => applyUserAvatar(qc, res.avatar_url),
    });
}

export function useDeleteUserAvatar() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: () => deleteUserAvatar(),
        onSuccess: () => applyUserAvatar(qc, null),
    });
}
