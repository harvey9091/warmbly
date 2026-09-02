// The same-origin JSON API the Go forms service (cmd/forms) exposes for this
// app. Mirrors internal/formserver: keep the shapes in sync with its
// public handlers.

export type FormFieldType =
    | "text"
    | "email"
    | "phone"
    | "textarea"
    | "number"
    | "select"
    | "radio"
    | "checkboxes"
    | "checkbox"
    | "date"
    | "hidden"
    | "heading"
    | "paragraph"
    | "divider"
    | "page_break";

export interface FormField {
    id: string;
    type: FormFieldType;
    label: string;
    placeholder?: string;
    help_text?: string;
    required: boolean;
    options?: string[];
    /** Hidden-field constant or paragraph body. */
    value?: string;
    /** "full" (default) or "half"; two half fields share a row. */
    width?: string;
    rows?: number;
}

export interface FormDesign {
    font_family?: string;
    page_background?: string;
    form_background?: string;
    text_color?: string;
    label_color?: string;
    input_background?: string;
    input_border_color?: string;
    input_text_color?: string;
    placeholder_color?: string;
    accent_color?: string;
    button_background?: string;
    button_text_color?: string;
    button_text?: string;
    button_size?: string;
    button_full_width?: boolean;
    border_radius?: number;
    max_width?: number;
    spacing?: string;
    shadow?: boolean;
    theme?: string;
    layout?: string;
    mode?: string;
    page_background_end?: string;
    align?: string;
    show_progress?: boolean;
}

export interface PublicForm {
    public_id: string;
    name: string;
    fields: FormField[];
    design: FormDesign;
    logo_url?: string;
    cover_url?: string;
    background_url?: string;
    captcha_site_key?: string;
    /** Present only when a valid personalized ?t= link opened the page. */
    prefill?: Record<string, string>;
    link_token?: string;
}

export interface SubmitPayload {
    answers: Record<string, string[]>;
    /** Honeypot value; a human never fills it. */
    website: string;
    /** Unix second the page was rendered; bots submit near-instantly. */
    _wt: number;
    captcha_token?: string;
    source_url?: string;
    /** Personalized link ticket; attributes the submission to a contact. */
    link_token?: string;
    visitor_key?: string;
}

export interface SubmitResult {
    message: string;
    redirect_url?: string;
}

export class FormNotFoundError extends Error {
    constructor() {
        super("form not found");
        this.name = "FormNotFoundError";
    }
}

export class SubmitRejectedError extends Error {}

/** The tab outlived its render token; only a reload can mint a new one. */
export class StalePageError extends Error {
    constructor() {
        super("stale page");
        this.name = "StalePageError";
    }
}

// The Go shell stamps the render token into this meta tag; every API call
// must carry it, so a script that never loaded the page gets nothing.
const renderToken =
    document.querySelector('meta[name="wf-token"]')?.getAttribute("content") ?? "";

export function apiHeaders(): Record<string, string> {
    return { Accept: "application/json", "X-Warmbly-Render": renderToken };
}

// personalToken reads the ?t= ticket from a personalized link.
export function personalToken(): string | null {
    const t = new URLSearchParams(window.location.search).get("t");
    return t && /^[A-Za-z0-9-]{1,64}$/.test(t) ? t : null;
}

async function throwForStatus(res: Response): Promise<never> {
    const body = (await res.json().catch(() => null)) as { error?: string; message?: string } | null;
    if (res.status === 403 && body?.error === "stale_page") throw new StalePageError();
    if (res.status === 404) throw new FormNotFoundError();
    throw new Error(`request failed (${res.status})`);
}

export async function fetchForm(publicId: string, linkToken?: string | null): Promise<PublicForm> {
    const qs = linkToken ? `?t=${encodeURIComponent(linkToken)}` : "";
    const res = await fetch(`/api/forms/${encodeURIComponent(publicId)}${qs}`, {
        headers: apiHeaders(),
    });
    if (!res.ok) await throwForStatus(res);
    return (await res.json()) as PublicForm;
}

export async function submitForm(publicId: string, payload: SubmitPayload): Promise<SubmitResult> {
    const res = await fetch(`/api/forms/${encodeURIComponent(publicId)}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...apiHeaders() },
        body: JSON.stringify(payload),
    });
    const body = (await res.json().catch(() => null)) as {
        error?: string;
        message?: string;
    } | null;
    if (res.status === 403 && body?.error === "stale_page") throw new StalePageError();
    if (res.status === 404) throw new FormNotFoundError();
    if (!res.ok) {
        throw new SubmitRejectedError(body?.message || "Something went wrong. Try again.");
    }
    return (body ?? { message: "Thanks!" }) as SubmitResult;
}
