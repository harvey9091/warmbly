// Vendor accounts and their mailboxes, under their own root so a mailbox
// invalidation does not refetch them; the mailbox_vendor audit spine keeps
// every teammate's view live.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    createVendorConnection,
    deleteVendorConnection,
    importVendorMailboxes,
    listVendorCatalog,
    listVendorConnections,
    listVendorMailboxes,
    updateVendorConnection,
} from "@/lib/api/client/app/emails/imports/mailboxVendors";
import type { MailboxImport } from "@/lib/api/models/app/emails/MailboxImport";
import type {
    CreateVendorRequest,
    ListResponse,
    UpdateVendorRequest,
    VendorConnection,
    VendorImportRequest,
} from "@/lib/api/models/app/emails/MailboxSources";
import { sourceMutationKey } from "./mailboxSourceBusy";
import { SENDING_DOMAINS_KEY } from "./useSendingDomains";

export const VENDORS_KEY = ["mailbox-vendors"] as const;

export function useVendorCatalog(enabled = true) {
    return useQuery({
        queryKey: [...VENDORS_KEY, "catalog"],
        queryFn: listVendorCatalog,
        enabled,
        staleTime: 60 * 60_000,
        retry: false,
    });
}

export function useVendorConnections(enabled = true) {
    return useQuery({
        queryKey: [...VENDORS_KEY, "list"],
        queryFn: listVendorConnections,
        enabled,
        staleTime: 15_000,
    });
}

// Each read asks the vendor's API, so it is not refetched on focus.
export function useVendorMailboxes(id: string | null) {
    return useQuery({
        queryKey: [...VENDORS_KEY, id, "mailboxes"],
        queryFn: () => listVendorMailboxes(id!),
        enabled: !!id,
        staleTime: 60_000,
        refetchOnWindowFocus: false,
        retry: false,
    });
}

// Writes the account into the list so the caller sees it before the refetch lands.
function useStoreConnection() {
    const qc = useQueryClient();
    return (conn: VendorConnection) => {
        qc.setQueryData<ListResponse<VendorConnection>>([...VENDORS_KEY, "list"], (prev) =>
            prev ? { data: [conn, ...prev.data.filter((c) => c.id !== conn.id)] } : prev,
        );
        qc.invalidateQueries({ queryKey: [...VENDORS_KEY, "list"] });
    };
}

export function useCreateVendorConnection() {
    const store = useStoreConnection();
    const qc = useQueryClient();
    return useMutation({
        mutationKey: sourceMutationKey("vendor-create"),
        mutationFn: (body: CreateVendorRequest) => createVendorConnection(body),
        onSuccess: (conn) => {
            store(conn);
            // A connected account can take over its domains' forwarding and DNS.
            qc.invalidateQueries({ queryKey: SENDING_DOMAINS_KEY });
        },
    });
}

export function useUpdateVendorConnection() {
    const qc = useQueryClient();
    const store = useStoreConnection();
    return useMutation({
        mutationKey: sourceMutationKey("vendor-update"),
        mutationFn: ({ id, body }: { id: string; body: UpdateVendorRequest }) => updateVendorConnection(id, body),
        onSuccess: (conn) => {
            store(conn);
            // A new key can see different mailboxes.
            qc.invalidateQueries({ queryKey: [...VENDORS_KEY, conn.id] });
        },
    });
}

export function useDeleteVendorConnection() {
    const qc = useQueryClient();
    return useMutation({
        mutationKey: sourceMutationKey("vendor-delete"),
        mutationFn: (id: string) => deleteVendorConnection(id),
        onSuccess: () => {
            qc.invalidateQueries({ queryKey: [...VENDORS_KEY, "list"] });
            qc.invalidateQueries({ queryKey: SENDING_DOMAINS_KEY });
        },
    });
}

export function useImportVendorMailboxes() {
    const qc = useQueryClient();
    return useMutation({
        mutationKey: sourceMutationKey("vendor-import"),
        mutationFn: ({ id, body }: { id: string; body: VendorImportRequest }) => importVendorMailboxes(id, body),
        onSuccess: (job) => {
            qc.setQueryData<MailboxImport>(["emails", "imports", job.id], job);
            qc.invalidateQueries({ queryKey: ["emails", "imports"] });
            qc.invalidateQueries({ queryKey: VENDORS_KEY });
        },
    });
}
