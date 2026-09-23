import Request from "../../../Request";
import type { MailboxImport } from "@/lib/api/models/app/emails/MailboxImport";

// Stops an import; rows already connected stay connected.
export default async function cancelMailboxImport(id: string): Promise<MailboxImport> {
    return await Request<MailboxImport>({
        method: "POST",
        url: `/emails/imports/${id}/cancel`,
        authorization: true,
    });
}
