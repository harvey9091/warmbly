import { useQuery } from "@tanstack/react-query";
import listMailboxImports from "@/lib/api/client/app/emails/imports/listMailboxImports";

export const MAILBOX_IMPORTS_KEY = ["emails", "imports"] as const;

// Recent imports for the mailboxes page; MAILBOX_IMPORT_PROGRESS keeps it live.
export default function useMailboxImports(enabled = true, limit = 10) {
    return useQuery({
        queryKey: [...MAILBOX_IMPORTS_KEY, "list", limit],
        queryFn: () => listMailboxImports({ limit }),
        enabled,
        staleTime: 15_000,
    });
}
