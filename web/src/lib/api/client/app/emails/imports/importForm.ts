import type { MailboxImportInput } from "@/lib/api/models/app/emails/MailboxImport";

// The multipart body preview and create share: a file or pasted text, plus the mapping and options as JSON.
export default function importForm(input: MailboxImportInput): FormData {
    const form = new FormData();
    if (input.file) form.append("file", input.file);
    else form.append("text", input.text ?? "");
    if (input.mapping && Object.keys(input.mapping).length > 0) form.append("mapping", JSON.stringify(input.mapping));
    if (input.options) form.append("options", JSON.stringify(input.options));
    return form;
}
