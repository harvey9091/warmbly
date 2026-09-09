import { describe, expect, it } from "vitest";
import { normalizePastedHTML } from "./pasteHtml";
import { htmlToPlain } from "./emailPreview";

describe("normalizePastedHTML", () => {
    it("drops the empty block Gmail writes a blank line as", () => {
        // Our paragraphs carry their own bottom margin, so keeping this one
        // renders the gap twice — the double spacing in issue #380.
        const out = normalizePastedHTML("<div>One</div><div><br></div><div>Two</div>");
        expect(out).toBe("<div>One</div><div>Two</div>");
    });

    it("drops Word's spacer paragraph and its namespaced tags", () => {
        const out = normalizePastedHTML(
            '<p class="MsoNormal">One<o:p></o:p></p>' +
                '<p class="MsoNormal"><o:p> </o:p></p>' +
                '<p class="MsoNormal">Two</p>',
        );
        expect(out).toBe('<p class="MsoNormal">One</p><p class="MsoNormal">Two</p>');
    });

    it("removes a stylesheet instead of letting its CSS land as copy", () => {
        const out = normalizePastedHTML("<style>p{color:red}</style><p>Hi</p>");
        expect(out).toBe("<p>Hi</p>");
    });

    it("collapses a run of breaks and strips the one a block ends with", () => {
        expect(normalizePastedHTML("<p>One<br><br>Two<br></p>")).toBe("<p>One<br>Two</p>");
    });

    it("keeps an image a mail client can load and removes one it cannot", () => {
        expect(normalizePastedHTML('<p><img src="https://x.test/a.png"></p>')).toContain("https://x.test/a.png");
        // A `cid:` part belongs to the message it was copied from, so the
        // paragraph holding it is empty once it goes and drops with it.
        expect(normalizePastedHTML('<p><img src="cid:part1"></p>')).toBe("");
    });

    it("keeps a block whose only content is an image", () => {
        const out = normalizePastedHTML('<div><img src="https://x.test/a.png"></div>');
        expect(out).toContain("<img");
    });

    it("unwraps styling-only elements but keeps our merge-field chips", () => {
        expect(normalizePastedHTML('<p><font color="red">Hi</font></p>')).toBe("<p>Hi</p>");
        const chip = '<p><span data-var="">{{.FirstName}}</span></p>';
        expect(normalizePastedHTML(chip)).toBe(chip);
    });

    it("leaves a copy from another TipTap editor untouched", () => {
        // ProseMirror marks its own clipboard HTML and re-parses it exactly.
        const html = '<div data-pm-slice="1 1 []"><p>One</p><p><br></p></div>';
        expect(normalizePastedHTML(html)).toBe(html);
    });
});

describe("htmlToPlain with images", () => {
    it("stands an image in for its alt text", () => {
        expect(htmlToPlain('<p>Look:</p><img src="https://x.test/a.png" alt="Our dashboard">')).toBe(
            "Look:\n[Our dashboard]",
        );
    });

    it("drops an image with no alt text rather than leaving brackets", () => {
        expect(htmlToPlain('<p>Hi</p><img src="https://x.test/a.png">')).toBe("Hi");
    });
});
