// Round-tripping real email markup through the editor schema.
//
// The schema is the contract for what survives being pasted or reopened, and
// anything it cannot represent is dropped silently (issue #393). These tests
// parse markup and serialize it back, which is exactly what the editor does on
// paste and on mount, so a regression here is a design quietly losing its
// layout rather than a failing render.

import { describe, expect, it } from "vitest";
import { Editor } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Text from "@tiptap/extension-text";
import Bold from "@tiptap/extension-bold";
import Italic from "@tiptap/extension-italic";
import Heading from "@tiptap/extension-heading";
import Link from "@tiptap/extension-link";
import HardBreak from "@tiptap/extension-hard-break";
import { BulletList, OrderedList, ListItem } from "@tiptap/extension-list";
import { emailDesignExtensions, EmailParagraph } from "./emailHtml";

const extensions = [
    Document,
    EmailParagraph,
    Text,
    Bold,
    Italic,
    HardBreak,
    Heading.configure({ levels: [2, 3] }),
    BulletList,
    OrderedList,
    ListItem,
    Link.configure({ openOnClick: false, autolink: true }),
    // The same list the editor mounts, so this cannot pass while the editor
    // drops the markup these tests exist to protect.
    ...emailDesignExtensions,
];

// A real editor, because that is what parses the body on paste and on mount.
function roundTrip(html: string): string {
    const editor = new Editor({ element: document.createElement("div"), extensions, content: html });
    const out = editor.getHTML();
    editor.destroy();
    return out;
}

describe("the email schema", () => {
    // TipTap's own table renderer injects a <colgroup> and a min-width. Both
    // are editor furniture, and a min-width nobody asked for turning up on a
    // template's 600px table is the editor mangling a design.
    it("keeps a table layout with its sizing and cell styles, and adds nothing", () => {
        const out = roundTrip(
            '<table width="600" cellpadding="0" cellspacing="0" bgcolor="#f4f4f4">' +
                '<tr><td style="padding:16px" valign="top" align="center"><p>Hi</p></td></tr></table>',
        );
        expect(out).toContain("<table");
        expect(out).toContain('width="600"');
        expect(out).toContain('cellpadding="0"');
        expect(out).toContain('bgcolor="#f4f4f4"');
        expect(out).toContain("padding: 16px");
        expect(out).toContain('valign="top"');
        expect(out).not.toContain("colgroup");
        expect(out).not.toContain("min-width");
    });

    it("keeps a div wrapper and the styles on it", () => {
        const out = roundTrip('<div class="wrap" style="max-width:600px;margin:0 auto"><p>Hi</p></div>');
        expect(out).toContain("<div");
        expect(out).toContain('class="wrap"');
        expect(out).toContain("max-width: 600px");
    });

    // A <div> of text becoming a <p> adds the client's paragraph margins, and
    // a template that spaced its own rows suddenly has gaps.
    it("keeps a text-only div a div", () => {
        const out = roundTrip('<div style="padding:8px">Hi Ana</div>');
        expect(out).toContain("<div");
        expect(out).not.toContain("<p");
        expect(out).toContain("padding: 8px");
    });

    it("keeps colour, highlight, font and size on a span", () => {
        const out = roundTrip(
            '<p><span style="color: #0284c7; background-color: #fef08a; ' +
                'font-family: Georgia, serif; font-size: 18px">Hi</span></p>',
        );
        // A colour comes back as the rgb() the browser normalises it to.
        for (const decl of [
            "color: rgb(2, 132, 199)",
            "background-color: rgb(254, 240, 138)",
            "font-family: Georgia",
            "font-size: 18px",
        ]) {
            expect(out).toContain(decl);
        }
    });

    // <p align="center"> is how mail has said this since 1997 and the
    // attribute is meaningless on a paragraph in modern HTML, so it is folded
    // into the style rather than dropped.
    it("keeps alignment, including the legacy attribute, alongside its own styles", () => {
        const out = roundTrip('<p style="padding:4px" align="center">Hi</p>');
        expect(out).toContain("text-align: center");
        expect(out).toContain("padding: 4px");
    });

    // TipTap merges two style attributes property by property, splitting each
    // declaration on the first colon. A value that contains one of its own has
    // to survive that, or every background image in a pasted design is
    // truncated to "url(https".
    it("does not truncate a style value that contains a colon", () => {
        const out = roundTrip(
            '<div style="background-image:url(https://cdn.test/a.png);text-align:right"><p>Hi</p></div>',
        );
        expect(out).toContain("https://cdn.test/a.png");
        expect(out).not.toContain("url(https)");
    });

    it("keeps the structure the visual editor already held", () => {
        const out = roundTrip("<h2>Title</h2><ul><li>One</li></ul><p><strong>Bold</strong> and <em>italic</em></p>");
        for (const tag of ["<h2>", "<ul>", "<li>", "<strong>", "<em>"]) {
            expect(out).toContain(tag);
        }
    });
});
