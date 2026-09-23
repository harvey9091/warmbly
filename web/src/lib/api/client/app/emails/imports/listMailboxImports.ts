import Request from "../../../Request";
import type { MailboxImportList } from "@/lib/api/models/app/emails/MailboxImport";

export default async function listMailboxImports(params: { cursor?: string; limit?: number } = {}): Promise<MailboxImportList> {
    return await Request<MailboxImportList>({
        method: "GET",
        url: `/emails/imports`,
        params: {
            cursor: params.cursor || undefined,
            limit: params.limit,
        },
        authorization: true,
    });
}
