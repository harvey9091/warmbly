import { describe, expect, it } from "vitest";
import { detectPastedEmail, isDocumentBody } from "./pastedEmail";

// A stand-in for the clipboard: only getData is read.
function clipboard(data: Partial<Record<"text/plain" | "text/html", string>>): DataTransfer {
    return { getData: (type: string) => data[type as keyof typeof data] ?? "" } as DataTransfer;
}

describe("detectPastedEmail", () => {
    it("adopts markup copied as text", () => {
        const source = '<!DOCTYPE html><html><body><table><tr><td>Hi</td></tr></table></body></html>';
        expect(detectPastedEmail(clipboard({ "text/plain": source }))).toBe(source);
        expect(detectPastedEmail(clipboard({ "text/plain": "<table><tr><td>Hi</td></tr></table>" }))).toBe(
            "<table><tr><td>Hi</td></tr></table>",
        );
    });

    it("adopts a whole HTML document from the clipboard's HTML flavour", () => {
        const doc = "<html><head><style>.x{color:red}</style></head><body><p>Hi</p></body></html>";
        expect(detectPastedEmail(clipboard({ "text/html": doc }))).toBe(doc);
    });

    // The bar has to stay high: a copy out of a browser, Gmail or Word is
    // wrapped in <html><body> too, and switching to a source view every time
    // someone pastes a sentence would be worse than the bug.
    it("leaves an ordinary copy out of a rendered page alone", () => {
        const browserCopy =
            "<html><body><!--StartFragment--><p>just a sentence</p><!--EndFragment--></body></html>";
        expect(detectPastedEmail(clipboard({ "text/plain": "just a sentence", "text/html": browserCopy }))).toBeNull();
    });

    it("leaves prose that merely mentions a tag alone", () => {
        expect(detectPastedEmail(clipboard({ "text/plain": "we ship <html> emails now" }))).toBeNull();
        expect(detectPastedEmail(clipboard({ "text/plain": "<table without a close" }))).toBeNull();
    });

    // A saved email opens with the tool that wrote it before its doctype.
    it("adopts a document behind a leading comment", () => {
        const doc = "<!-- saved from Mailchimp --><!DOCTYPE html><html><body><p>Hi</p></body></html>";
        expect(detectPastedEmail(clipboard({ "text/html": doc }))).toBe(doc);
    });

    it("is null for an empty clipboard", () => {
        expect(detectPastedEmail(null)).toBeNull();
        expect(detectPastedEmail(clipboard({}))).toBeNull();
    });
});

describe("isDocumentBody", () => {
    it("is true for markup no schema can hold faithfully", () => {
        expect(isDocumentBody("<!doctype html><html><body>x</body></html>")).toBe(true);
        expect(isDocumentBody('<style>.x{color:red}</style><p class="x">x</p>')).toBe(true);
    });

    it("sees a document behind a leading comment", () => {
        expect(isDocumentBody("<!-- saved from Mailchimp --><!doctype html><html><body>x</body></html>")).toBe(true);
    });

    it("is false for an ordinary campaign body", () => {
        expect(isDocumentBody("<p>Hi {{.FirstName}}, do you have ten minutes?</p>")).toBe(false);
        expect(isDocumentBody('<table><tr><td style="padding:8px">Hi</td></tr></table>')).toBe(false);
    });
});
