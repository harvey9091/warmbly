import { keepPreviousData, useQuery } from "@tanstack/react-query";
import listMailboxImportRows from "@/lib/api/client/app/emails/imports/listMailboxImportRows";
import type { ImportRowsParams } from "@/lib/api/models/app/emails/MailboxImport";

// One page of an import's rows. Under ["emails","imports",id] so the job's events refresh it.
export default function useMailboxImportRows(id: string | null, params: ImportRowsParams, running = false) {
    return useQuery({
        queryKey: ["emails", "imports", id, "rows", params],
        queryFn: () => listMailboxImportRows(id!, params),
        enabled: !!id,
        staleTime: 2_000,
        placeholderData: keepPreviousData,
        refetchInterval: running ? 5_000 : false,
    });
}
