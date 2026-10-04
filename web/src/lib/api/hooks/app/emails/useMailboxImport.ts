import { useQuery } from "@tanstack/react-query";
import getMailboxImport from "@/lib/api/client/app/emails/imports/getMailboxImport";

// One import job. Realtime drives it; the 5s poll is a fallback only while it runs.
export default function useMailboxImport(id: string | null) {
    return useQuery({
        queryKey: ["emails", "imports", id],
        queryFn: () => getMailboxImport(id!),
        enabled: !!id,
        staleTime: 2_000,
        refetchInterval: (query) => (query.state.data?.status === "running" ? 5_000 : false),
    });
}
