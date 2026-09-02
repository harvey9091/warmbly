// MIRROR — this file exists byte-identical in web/src/components/app/forms/
// and forms/src/. Edit both copies together; scripts/check-forms-mirror.sh
// (run by `make lint`) fails the build when they drift. It is self-contained
// on purpose: the builder canvas and the hosted page must resolve a design
// and split pages exactly the same way, and this file is that single truth.

export interface DesignInput {
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
    /** "cover" (fill), "contain" (fit) or "tile" (repeat). */
    background_size?: string;
    /** 0-100: how much the page color veils the image, for readable text. */
    background_overlay?: number;
    /** A bar carrying the logo and a title. */
    header_enabled?: boolean;
    header_title?: string;
    header_background?: string;
    /** "page" spans the viewport, "inline" sits with the form itself. */
    header_placement?: string;
    /** "left", "center" or "between"; defaults to the form's alignment. */
    header_align?: string;
    /** Keep a page-width header visible while the form scrolls. */
    header_sticky?: boolean;
    /** false leaves the logo in its own placement and gives the header the
     *  title alone. Defaults to true. */
    header_show_logo?: boolean;
    /** Text over the Split layout's cover panel. */
    cover_title?: string;
    cover_subtitle?: string;
    /** "sm" | "md" | "lg": how tall the logo renders. */
    logo_size?: string;
    /** "card" sits it on the form surface, "page" above the card. */
    logo_position?: string;
}

// CoreField is the structural slice of a form field the page math needs;
// both trees' FormField types satisfy it.
export interface CoreField {
    id: string;
    type: string;
    label: string;
}

export type FormLayout = "card" | "wide" | "split";
export type FormMode = "classic" | "focus";
export type FormAlign = "left" | "center";
export type FormBackgroundSize = "cover" | "contain" | "tile";
export type HeaderPlacement = "page" | "inline";
export type HeaderAlign = "left" | "center" | "between";

// Keep in sync with the font_family switch in internal/models/form.go.
export const FONT_CATALOG: Record<string, { label: string; stack: string; google?: string }> = {
    system: { label: "System", stack: "system-ui,-apple-system,'Segoe UI',Roboto,sans-serif" },
    inter: {
        label: "Inter",
        stack: "'Inter',system-ui,-apple-system,'Segoe UI',sans-serif",
        google: "https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap",
    },
    serif: { label: "Serif", stack: "Georgia,'Times New Roman',serif" },
    mono: { label: "Mono", stack: "ui-monospace,SFMono-Regular,Menlo,monospace" },
    manrope: {
        label: "Manrope",
        stack: "'Manrope',system-ui,sans-serif",
        google: "https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700&display=swap",
    },
    sora: {
        label: "Sora",
        stack: "'Sora',system-ui,sans-serif",
        google: "https://fonts.googleapis.com/css2?family=Sora:wght@400;500;600&display=swap",
    },
    fraunces: {
        label: "Fraunces",
        stack: "'Fraunces',Georgia,serif",
        google: "https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,400;9..144,500;9..144,600&display=swap",
    },
    "space-grotesk": {
        label: "Space Grotesk",
        stack: "'Space Grotesk',system-ui,sans-serif",
        google: "https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600&display=swap",
    },
};

export interface ResolvedDesign {
    font: string;
    fontStack: string;
    googleFontUrl: string | null;
    pageBg: string;
    pageBg2: string | null;
    /** Solid color, or a vertical gradient when page_background_end is set. */
    pageBgCss: string;
    formBg: string;
    text: string;
    label: string;
    inputBg: string;
    inputBorder: string;
    inputText: string;
    placeholder: string;
    accent: string;
    btnBg: string;
    btnText: string;
    btnLabel: string;
    btnPad: string;
    btnFont: number;
    btnFullWidth: boolean;
    radius: number;
    inputRadius: number;
    maxWidth: number;
    gap: number;
    shadow: boolean;
    layout: FormLayout;
    mode: FormMode;
    align: FormAlign;
    showProgress: boolean;
    bgSize: FormBackgroundSize;
    /** 0..1, already normalized from the 0-100 the editor stores. */
    bgOverlay: number;
    headerEnabled: boolean;
    headerTitle: string;
    headerBg: string;
    headerPlacement: HeaderPlacement;
    headerAlign: HeaderAlign;
    headerSticky: boolean;
    headerShowLogo: boolean;
    coverTitle: string;
    coverSubtitle: string;
    logoHeight: number;
    /** true renders the logo above the card, on the page background. */
    logoOnPage: boolean;
}

export function resolveDesign(d: DesignInput): ResolvedDesign {
    const font = d.font_family && FONT_CATALOG[d.font_family] ? d.font_family : "system";
    const radius = d.border_radius ?? 10;
    const btnSizes: Record<string, [string, number]> = {
        sm: ["8px 16px", 13],
        md: ["10px 20px", 14],
        lg: ["13px 26px", 16],
    };
    const [btnPad, btnFont] = btnSizes[d.button_size ?? "md"] ?? btnSizes.md;
    const pageBg = d.page_background || "#f8fafc";
    const pageBg2 = d.page_background_end || null;
    const layout: FormLayout = d.layout === "wide" || d.layout === "split" ? d.layout : "card";
    return {
        font,
        fontStack: FONT_CATALOG[font].stack,
        googleFontUrl: FONT_CATALOG[font].google ?? null,
        pageBg,
        pageBg2,
        pageBgCss: pageBg2 ? `linear-gradient(180deg, ${pageBg}, ${pageBg2})` : pageBg,
        formBg: d.form_background || "#ffffff",
        text: d.text_color || "#0f172a",
        label: d.label_color || "#334155",
        inputBg: d.input_background || "#ffffff",
        inputBorder: d.input_border_color || "#e2e8f0",
        inputText: d.input_text_color || "#0f172a",
        placeholder: d.placeholder_color || "#94a3b8",
        accent: d.accent_color || "#0284c7",
        btnBg: d.button_background || "#0284c7",
        btnText: d.button_text_color || "#ffffff",
        btnLabel: d.button_text || "Submit",
        btnPad,
        btnFont,
        btnFullWidth: !!d.button_full_width,
        radius,
        inputRadius: Math.min(radius, 12),
        maxWidth: d.max_width ?? 560,
        gap: d.spacing === "compact" ? 10 : d.spacing === "relaxed" ? 22 : 16,
        // Only the card layout carries a box that can cast a shadow.
        shadow: (d.shadow ?? true) && layout === "card",
        layout,
        mode: d.mode === "focus" ? "focus" : "classic",
        align: d.align === "center" ? "center" : "left",
        showProgress: d.show_progress ?? true,
        bgSize: d.background_size === "contain" || d.background_size === "tile" ? d.background_size : "cover",
        // Clamped here so a bad stored value can never wash the page out.
        bgOverlay: Math.min(100, Math.max(0, d.background_overlay ?? 0)) / 100,
        headerEnabled: !!d.header_enabled,
        headerTitle: d.header_title || "",
        headerPlacement: d.header_placement === "inline" ? "inline" : "page",
        // Unset follows the form's own alignment, so a centered form gets a
        // centered header without a second decision.
        headerAlign:
            d.header_align === "left" || d.header_align === "center" || d.header_align === "between"
                ? d.header_align
                : d.align === "center"
                  ? "center"
                  : "left",
        // Sticky only means anything for a page-width bar.
        headerSticky: !!d.header_sticky && d.header_placement !== "inline",
        headerShowLogo: d.header_show_logo ?? true,
        // The header sits on the form surface by default so it reads as chrome
        // rather than a second background.
        headerBg: d.header_background || d.form_background || "#ffffff",
        coverTitle: d.cover_title || "",
        coverSubtitle: d.cover_subtitle || "",
        logoHeight: { sm: 24, md: 36, lg: 52 }[d.logo_size ?? "md"] ?? 36,
        // Only the card layout has a box to sit above; wide and split put the
        // logo on the form surface either way.
        logoOnPage: d.logo_position === "page" && (d.layout ?? "card") === "card",
    };
}

// designVars is the full --wf-* variable map form-theme.css reads. The hosted
// page paints it onto documentElement, the builder canvas spreads it as an
// inline style; the shared name list is what keeps the two from drifting.
export function designVars(r: ResolvedDesign): Record<string, string> {
    return {
        "--wf-font": r.fontStack,
        "--wf-page-bg": r.pageBg,
        "--wf-page-bg-css": r.pageBgCss,
        "--wf-form-bg": r.formBg,
        "--wf-text": r.text,
        "--wf-label": r.label,
        "--wf-input-bg": r.inputBg,
        "--wf-input-border": r.inputBorder,
        "--wf-input-text": r.inputText,
        "--wf-placeholder": r.placeholder,
        "--wf-accent": r.accent,
        "--wf-btn-bg": r.btnBg,
        "--wf-btn-text": r.btnText,
        "--wf-btn-pad": r.btnPad,
        "--wf-btn-font": `${r.btnFont}px`,
        "--wf-radius": `${r.radius}px`,
        "--wf-input-radius": `${r.inputRadius}px`,
        "--wf-max-width": `${r.maxWidth}px`,
        "--wf-gap": `${r.gap}px`,
        "--wf-shadow": r.shadow ? "0 1px 2px rgba(15,23,42,.06),0 8px 24px rgba(15,23,42,.08)" : "none",
        "--wf-bg-size": r.bgSize === "tile" ? "auto" : r.bgSize,
        "--wf-bg-repeat": r.bgSize === "tile" ? "repeat" : "no-repeat",
        "--wf-bg-overlay": String(r.bgOverlay),
        "--wf-header-bg": r.headerBg,
        "--wf-logo-h": `${r.logoHeight}px`,
    };
}

// ensureFont loads the resolved Google font once per document.
export function ensureFont(r: ResolvedDesign) {
    if (!r.googleFontUrl) return;
    const id = `wf-font-${r.font}`;
    if (document.getElementById(id)) return;
    const link = document.createElement("link");
    link.id = id;
    link.rel = "stylesheet";
    link.href = r.googleFontUrl;
    document.head.appendChild(link);
}

export interface FormPage<F extends CoreField> {
    title: string;
    fields: F[];
}

const isVisibleInput = (f: CoreField) =>
    !["heading", "paragraph", "divider", "page_break", "hidden"].includes(f.type);

// splitPages cuts the flat field list on page_break blocks. The break's label
// titles the page it OPENS; pages left with no fields at all are dropped so a
// stray leading or trailing break cannot render an empty screen.
export function splitPages<F extends CoreField>(fields: F[]): FormPage<F>[] {
    const pages: FormPage<F>[] = [{ title: "", fields: [] }];
    for (const f of fields) {
        if (f.type === "page_break") {
            pages.push({ title: f.label ?? "", fields: [] });
        } else {
            pages[pages.length - 1].fields.push(f);
        }
    }
    const filled = pages.filter((p) => p.fields.length > 0);
    return filled.length > 0 ? filled : [pages[0]];
}

// focusSteps groups the list for focus mode: every visible input anchors one
// screen, carrying the layout blocks and hidden fields written before it;
// trailing blocks join the last screen. Page breaks are boundaries only.
export function focusSteps<F extends CoreField>(fields: F[]): F[][] {
    const steps: F[][] = [];
    let pending: F[] = [];
    for (const f of fields) {
        if (f.type === "page_break") continue;
        pending.push(f);
        if (isVisibleInput(f)) {
            steps.push(pending);
            pending = [];
        }
    }
    if (pending.length > 0) {
        if (steps.length > 0) steps[steps.length - 1].push(...pending);
        else steps.push(pending);
    }
    return steps;
}

// pageIndexOf maps every field id to the classic page it lives on, so focus
// mode can report funnel progress in page terms.
export function pageIndexOf(fields: CoreField[]): Record<string, number> {
    const out: Record<string, number> = {};
    let page = 0;
    for (const f of fields) {
        if (f.type === "page_break") {
            page += 1;
        } else {
            out[f.id] = page;
        }
    }
    return out;
}
