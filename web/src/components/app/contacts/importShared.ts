// Shared, non-component pieces of the contact-import flow. Kept in their own
// module (not exported from ImportWizard.tsx) so both the CSV ImportWizard and
// the Google-Sheet SheetSyncWizard can reuse the exact same column targets,
// dedup options, and error formatter without tripping react-refresh's
// "only export components" rule.

import toast from "react-hot-toast";
import type {
    ImportColumnMapping,
    ImportDedupStrategy,
    ImportResult,
} from "@/lib/api/client/app/contacts/importContacts";

export const STANDARD_TARGETS: { id: string; label: string }[] = [
    { id: "ignore", label: "Ignore" },
    { id: "email", label: "Email" },
    { id: "first_name", label: "First name" },
    { id: "last_name", label: "Last name" },
    { id: "company", label: "Company" },
    { id: "phone", label: "Phone" },
    { id: "subscribed", label: "Subscribed" },
    { id: "categories", label: "Categories" },
    { id: "verification_status", label: "Verification status" },
];

// Vocabularies the verification_status target can read, for the mapping
// row's "recognised as" badge. Mirrors emailverify.KnownVocabulary.
export const VERIFICATION_VOCABULARY_LABELS: Record<string, string> = {
    zerobounce: "ZeroBounce",
    millionverifier: "MillionVerifier",
    cleanmylist: "CleanMyList",
    neverbounce: "NeverBounce",
    bouncer: "Bouncer",
    kickbox: "Kickbox",
    emailable: "Emailable",
    debounce: "DeBounce",
    clearout: "Clearout",
    emaillistverify: "EmailListVerify",
    builtin: "Warmbly",
};

export const DEDUP_OPTIONS: { id: ImportDedupStrategy; label: string; hint: string }[] = [
    { id: "skip", label: "Skip existing", hint: "If a contact with this email exists, leave it alone." },
    { id: "update", label: "Update existing", hint: "Merge new values onto the existing contact." },
    {
        id: "create_duplicate",
        label: "Create duplicates",
        hint: "Force a new contact. Falls back to update if blocked by uniqueness.",
    },
];

// Extract a human-readable message from whatever the API client throws.
// Client.ts rethrows AppError (a plain object), not an Error instance —
// so `err instanceof Error` silently fails and you lose the real reason.
export function describeError(err: unknown, fallback: string): string {
    if (err && typeof err === "object") {
        const e = err as { message?: unknown; error?: unknown; status?: unknown };
        const msg = typeof e.message === "string" ? e.message.trim() : "";
        const title = typeof e.error === "string" ? e.error.trim() : "";
        const status = typeof e.status === "number" ? e.status : undefined;
        if (msg && title && msg !== title) {
            return status ? `${status} ${title}: ${msg}` : `${title}: ${msg}`;
        }
        if (msg) return status ? `${status}: ${msg}` : msg;
        if (title) return status ? `${status} ${title}` : title;
    }
    if (err instanceof Error && err.message) return err.message;
    return fallback;
}

// announceResult toasts a contact-import / sheet-sync result with the standard
// imported/updated/skipped summary (or a warning when rows failed).
export function announceResult(res: ImportResult) {
    if (res.failed === 0) {
        toast.success(
            `Imported ${res.imported} · updated ${res.updated} · skipped ${res.skipped}`,
        );
    } else {
        toast(`Synced with ${res.failed} errors`, { icon: "⚠️" });
    }
}

// ----- Custom-field names -----------------------------------------
//
// Mirrors internal/utils.IsValidJSONKey. A custom field is addressable in
// campaign copy either as {{.Role}} or, for a spaced/dashed name, through the
// server-side rewrite to (index . "Company Mobile"). Anything else would make
// a field the user can store but never merge into an email, so the API rejects
// it — we catch it here so a mistyped name never costs a whole import.
const CUSTOM_KEY_RE = /^[A-Za-z0-9_]+(?:[ -]+[A-Za-z0-9_]+)*$/;

export const CUSTOM_KEY_RULES = "Use letters, numbers, underscores, spaces or dashes.";

export function normalizeCustomKey(key: string): string {
    return key.trim().split(/\s+/).filter(Boolean).join(" ");
}

export function isValidCustomKey(key: string): boolean {
    const k = normalizeCustomKey(key);
    return k.length > 0 && k.length <= 255 && CUSTOM_KEY_RE.test(k);
}

// suggestCustomKey turns a raw spreadsheet header into a name the API accepts,
// so picking "Use as custom field" on a "Company Mobile" column just works.
// Returns "" when nothing usable survives and the user has to type a name.
export function suggestCustomKey(header: string): string {
    const cleaned = normalizeCustomKey(header.replace(/[^A-Za-z0-9_ -]+/g, " "))
        .replace(/^[-\s]+/, "")
        .replace(/[-\s]+$/, "");
    return isValidCustomKey(cleaned) ? cleaned : "";
}

export function isCustomTarget(target: string): boolean {
    return target === "custom" || target.startsWith("custom:");
}

// mappingProblem returns the first reason the mapping cannot be committed, or
// null when it is good to go. Same order of checks as the server so the two
// never disagree about which column is at fault.
export function mappingProblem(mapping: ImportColumnMapping[]): string | null {
    for (const m of mapping) {
        if (!isCustomTarget(m.target)) continue;
        const key = m.custom_key ?? (m.target.startsWith("custom:") ? m.target.slice(7) : "");
        if (normalizeCustomKey(key) === "") {
            return `Column ${m.index + 1} needs a custom field name.`;
        }
        if (!isValidCustomKey(key)) {
            return `"${key.trim()}" is not a valid field name. ${CUSTOM_KEY_RULES}`;
        }
    }
    if (!mapping.some((m) => m.target === "email")) {
        return "Map a column to Email.";
    }
    return null;
}
