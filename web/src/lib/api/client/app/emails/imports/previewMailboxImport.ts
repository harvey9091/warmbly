import Request from "../../../Request";
import type { MailboxImportInput, MailboxImportPreview } from "@/lib/api/models/app/emails/MailboxImport";
import importForm from "./importForm";

// The client revives ISO-looking strings into Dates; cell text from a file stays text.
function cellText(v: unknown): string {
    if (typeof v === "string") return v;
    if (v instanceof Date) return Number.isNaN(v.getTime()) ? "" : v.toISOString();
    return v == null ? "" : String(v);
}

// Reads a file or pasted list and reports what an import of it would do. Writes nothing.
export default async function previewMailboxImport(input: MailboxImportInput): Promise<MailboxImportPreview> {
    const p = await Request<MailboxImportPreview>({
        method: "POST",
        url: `/emails/imports/preview`,
        data: importForm(input),
        authorization: true,
        // DNS detection for every domain in the file happens before the answer.
        timeout: 60_000,
    });
    return {
        ...p,
        mapping: p.mapping ?? {},
        columns: (p.columns ?? []).map((c) => ({
            ...c,
            header: cellText(c.header),
            samples: Array.isArray(c.samples) ? c.samples.map(cellText) : [],
        })),
        rows: (p.rows ?? []).map((r) => ({ ...r, email: cellText(r.email), name: cellText(r.name) })),
        domains: p.domains ?? [],
        issues: p.issues ?? [],
    };
}
