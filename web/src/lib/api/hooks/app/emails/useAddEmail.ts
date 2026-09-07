import addEmail from "@/lib/api/client/app/emails/addEmail";
import type AddEmail from "@/lib/api/models/app/emails/AddEmail";
import { capture } from "@/lib/productAnalytics";
import { useMutation, useQueryClient } from "@tanstack/react-query";

// providerOf labels a manually connected mailbox by the mail host it uses, so
// the product event can say "another Google Workspace mailbox" without the
// event carrying the customer's mail infrastructure. Anything not on this list
// is reported as "other"; the raw host is never sent.
function providerOf(host: string): string {
    const h = host.trim().toLowerCase();
    if (h.endsWith("google.com") || h.endsWith("gmail.com")) return "google";
    if (h.endsWith("outlook.com") || h.endsWith("office365.com") || h.endsWith("microsoft.com")) return "microsoft";
    if (h.endsWith("zoho.com") || h.endsWith("zoho.eu")) return "zoho";
    if (h.endsWith("yahoo.com")) return "yahoo";
    if (h.endsWith("mail.ru") || h.endsWith("yandex.com")) return "other";
    return "other";
}

export default function useAddEmail() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (email: AddEmail) => addEmail(email),
        onSuccess: (_data, variables) => {
            queryClient.invalidateQueries({
                queryKey: ["emails", "list"]
            })
            capture("mailbox_connected", { provider: providerOf(variables.imap.host), method: "imap" });
        }
    })
}
