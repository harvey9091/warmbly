import Request from "../../../Request";
import type { MailboxImport, RetryImportRequest } from "@/lib/api/models/app/emails/MailboxImport";

// Queues failed rows again: every retryable one, one cause, or named lines, optionally with a new password.
export default async function retryMailboxImport(id: string, body: RetryImportRequest = {}): Promise<MailboxImport> {
    return await Request<MailboxImport>({
        method: "POST",
        url: `/emails/imports/${id}/retry`,
        data: body,
        authorization: true,
    });
}
