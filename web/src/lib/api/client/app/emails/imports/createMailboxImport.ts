import Request from "../../../Request";
import type { MailboxImport, MailboxImportInput } from "@/lib/api/models/app/emails/MailboxImport";
import importForm from "./importForm";

// Starts an import job; the backend connects the rows in the background.
export default async function createMailboxImport(input: MailboxImportInput): Promise<MailboxImport> {
    return await Request<MailboxImport>({
        method: "POST",
        url: `/emails/imports`,
        data: importForm(input),
        authorization: true,
        timeout: 60_000,
    });
}
