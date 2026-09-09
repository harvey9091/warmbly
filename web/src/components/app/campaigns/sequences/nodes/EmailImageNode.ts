// The <img> node for the campaign body editor (issue #380).
//
// Mail clients ignore stylesheets and half of them ignore <style> blocks too,
// so every layout decision has to survive as an inline style or an attribute on
// the tag itself. Width is written as both (Outlook reads the attribute), and
// alignment as auto margins on a block image, which is the one centring trick
// every client honours.

import { mergeAttributes } from "@tiptap/core";
import Image from "@tiptap/extension-image";

export type ImageAlign = "left" | "center" | "right";

// The width a body renders at in most mail clients; the size presets are
// fractions of it, so "L" fills the column instead of overflowing it.
export const EMAIL_BODY_WIDTH = 600;

export const IMAGE_SIZE_PRESETS: { label: string; title: string; width: number | null }[] = [
    { label: "S", title: "Quarter width", width: Math.round(EMAIL_BODY_WIDTH * 0.25) },
    { label: "M", title: "Half width", width: Math.round(EMAIL_BODY_WIDTH * 0.5) },
    { label: "L", title: "Full width", width: EMAIL_BODY_WIDTH },
    { label: "Auto", title: "The image's own size", width: null },
];

function readWidth(el: HTMLElement): number | null {
    const raw = el.getAttribute("width") || el.style.width || "";
    const n = Number.parseInt(raw, 10);
    return Number.isFinite(n) && n > 0 ? n : null;
}

export const EmailImage = Image.extend({
    addAttributes() {
        return {
            ...this.parent?.(),
            width: {
                default: null,
                parseHTML: (el) => readWidth(el as HTMLElement),
                // Composed into the tag's style + width by renderHTML below.
                renderHTML: () => ({}),
            },
            // A stale height fights `height:auto` when a client scales the
            // image down to the screen, so it is never carried.
            height: {
                default: null,
                parseHTML: () => null,
                renderHTML: () => ({}),
            },
            align: {
                default: "left" as ImageAlign,
                parseHTML: (el) => {
                    const a = (el as HTMLElement).getAttribute("data-align");
                    return a === "center" || a === "right" ? a : "left";
                },
                renderHTML: (attrs) => ({ "data-align": (attrs.align as ImageAlign) ?? "left" }),
            },
        };
    },

    renderHTML({ node, HTMLAttributes }) {
        const width = typeof node.attrs.width === "number" ? node.attrs.width : null;
        const align = (node.attrs.align as ImageAlign) ?? "left";
        const style = ["display:block", "max-width:100%", "height:auto", "border:0"];
        if (width) style.push(`width:${width}px`);
        style.push(align === "center" ? "margin:0 auto" : align === "right" ? "margin:0 0 0 auto" : "margin:0 auto 0 0");

        const extra: Record<string, string> = { style: style.join(";") };
        if (width) extra.width = String(width);
        return ["img", mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, extra)];
    },
});

export default EmailImage;
