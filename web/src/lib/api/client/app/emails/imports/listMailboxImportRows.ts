import Request from "../../../Request";
import type { ImportRowList, ImportRowsParams } from "@/lib/api/models/app/emails/MailboxImport";

export default async function listMailboxImportRows(id: string, params: ImportRowsParams = {}): Promise<ImportRowList> {
    return await Request<ImportRowList>({
        method: "GET",
        url: `/emails/imports/${id}/rows`,
        params: {
            status: params.status || undefined,
            cause: params.cause || undefined,
            cursor: params.cursor || undefined,
            limit: params.limit,
        },
        authorization: true,
    });
}
