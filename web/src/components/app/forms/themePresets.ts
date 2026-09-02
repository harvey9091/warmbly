// Curated one-click looks for the Design panel. A preset PATCHES concrete
// design keys (plus theme: id so the grid shows the selection); the renderer
// never reads the preset table, so shipping a new look is frontend-only.

import type { FormDesign } from "@/lib/api/models/app/forms/Form";

export interface ThemePreset {
    id: string;
    label: string;
    patch: Partial<FormDesign>;
}

const colorKeys = [
    "page_background",
    "page_background_end",
    "form_background",
    "text_color",
    "label_color",
    "input_background",
    "input_border_color",
    "input_text_color",
    "placeholder_color",
    "accent_color",
    "button_background",
    "button_text_color",
] as const;

export const THEME_PRESETS: ThemePreset[] = [
    {
        id: "minimal",
        label: "Minimal",
        patch: {
            font_family: "system",
            page_background: "#ffffff",
            form_background: "#ffffff",
            text_color: "#0f172a",
            label_color: "#334155",
            input_background: "#ffffff",
            input_border_color: "#e2e8f0",
            input_text_color: "#0f172a",
            placeholder_color: "#94a3b8",
            accent_color: "#0f172a",
            button_background: "#0f172a",
            button_text_color: "#ffffff",
            border_radius: 8,
            shadow: false,
        },
    },
    {
        id: "soft-slate",
        label: "Soft slate",
        patch: {
            font_family: "inter",
            page_background: "#f1f5f9",
            form_background: "#ffffff",
            text_color: "#0f172a",
            label_color: "#475569",
            input_background: "#f8fafc",
            input_border_color: "#e2e8f0",
            input_text_color: "#0f172a",
            placeholder_color: "#94a3b8",
            accent_color: "#475569",
            button_background: "#334155",
            button_text_color: "#ffffff",
            border_radius: 10,
            shadow: true,
        },
    },
    {
        id: "midnight",
        label: "Midnight",
        patch: {
            font_family: "inter",
            page_background: "#0f172a",
            form_background: "#1e293b",
            text_color: "#f1f5f9",
            label_color: "#cbd5e1",
            input_background: "#0f172a",
            input_border_color: "#334155",
            input_text_color: "#f1f5f9",
            placeholder_color: "#64748b",
            accent_color: "#38bdf8",
            button_background: "#38bdf8",
            button_text_color: "#0f172a",
            border_radius: 12,
            shadow: false,
        },
    },
    {
        id: "ocean",
        label: "Ocean",
        patch: {
            font_family: "sora",
            page_background: "#0c4a6e",
            page_background_end: "#082f49",
            form_background: "#ffffff",
            text_color: "#0f172a",
            label_color: "#334155",
            input_background: "#ffffff",
            input_border_color: "#e2e8f0",
            input_text_color: "#0f172a",
            placeholder_color: "#94a3b8",
            accent_color: "#0284c7",
            button_background: "#0284c7",
            button_text_color: "#ffffff",
            border_radius: 12,
            shadow: true,
        },
    },
    {
        id: "sunset",
        label: "Sunset",
        patch: {
            font_family: "manrope",
            page_background: "#fb923c",
            page_background_end: "#db2777",
            form_background: "#ffffff",
            text_color: "#1e1b4b",
            label_color: "#44403c",
            input_background: "#ffffff",
            input_border_color: "#fde3d3",
            input_text_color: "#1e1b4b",
            placeholder_color: "#a8a29e",
            accent_color: "#db2777",
            button_background: "#db2777",
            button_text_color: "#ffffff",
            border_radius: 14,
            shadow: true,
        },
    },
    {
        id: "paper",
        label: "Paper",
        patch: {
            font_family: "fraunces",
            page_background: "#faf7f2",
            form_background: "#faf7f2",
            text_color: "#1c1917",
            label_color: "#44403c",
            input_background: "#ffffff",
            input_border_color: "#d6d3d1",
            input_text_color: "#1c1917",
            placeholder_color: "#a8a29e",
            accent_color: "#9a3412",
            button_background: "#1c1917",
            button_text_color: "#faf7f2",
            border_radius: 4,
            shadow: false,
        },
    },
    {
        id: "bold",
        label: "Bold",
        patch: {
            font_family: "space-grotesk",
            page_background: "#f5f3ff",
            form_background: "#ffffff",
            text_color: "#1e1b4b",
            label_color: "#4c1d95",
            input_background: "#ffffff",
            input_border_color: "#ddd6fe",
            input_text_color: "#1e1b4b",
            placeholder_color: "#a5b4fc",
            accent_color: "#7c3aed",
            button_background: "#7c3aed",
            button_text_color: "#ffffff",
            button_full_width: true,
            border_radius: 14,
            shadow: true,
        },
    },
    {
        id: "glass",
        label: "Glass",
        patch: {
            font_family: "manrope",
            page_background: "#e0f2fe",
            page_background_end: "#ede9fe",
            form_background: "#ffffff",
            text_color: "#0f172a",
            label_color: "#334155",
            input_background: "#f8fafc",
            input_border_color: "#e2e8f0",
            input_text_color: "#0f172a",
            placeholder_color: "#94a3b8",
            accent_color: "#6366f1",
            button_background: "#6366f1",
            button_text_color: "#ffffff",
            border_radius: 16,
            shadow: true,
        },
    },
];

// applyThemePreset merges a preset over the current design, preserving the
// structural knobs (layout, mode, pages, sizing, copy): presets are looks.
export function applyThemePreset(current: FormDesign, preset: ThemePreset): FormDesign {
    const next: FormDesign = { ...current, ...preset.patch, theme: preset.id };
    // A preset without a gradient clears a lingering one, so switching from
    // Sunset to Minimal does not keep the pink fade.
    for (const key of colorKeys) {
        if (!(key in preset.patch)) delete next[key];
    }
    next.layout = current.layout;
    next.mode = current.mode;
    next.align = current.align;
    next.show_progress = current.show_progress;
    next.max_width = current.max_width;
    next.spacing = current.spacing;
    next.button_text = current.button_text;
    next.button_size = current.button_size;
    return next;
}
