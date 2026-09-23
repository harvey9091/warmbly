import Request from "../../../Request";
import type { MailboxImport } from "@/lib/api/models/app/emails/MailboxImport";

export default async function getMailboxImport(id: string): Promise<MailboxImport> {
    return await Request<MailboxImport>({
        method: "GET",
        url: `/emails/imports/${id}`,
        authorization: true,
    });
}
