import Request from "../../../Request";
import type { ImportRow, RowFix } from "@/lib/api/models/app/emails/MailboxImport";

// Replaces one row's credentials or servers and queues it again.
export default async function fixMailboxImportRow(id: string, line: number, fix: RowFix): Promise<ImportRow> {
    return await Request<ImportRow>({
        method: "PATCH",
        url: `/emails/imports/${id}/rows/${line}`,
        data: fix,
        authorization: true,
    });
}
