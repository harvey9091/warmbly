// Recognising a paste that is a whole HTML email rather than prose.
//
// A designed email is a document: its own <head>, a <style> block, nested
// tables. No editor schema can hold that faithfully, so parsing it is the
// silent gutting issue #393 reports. When we can tell that is what arrived,
// the body switches to HTML mode and keeps the markup exactly as written.
//
// The bar for switching is deliberately high. A copy out of a browser, Gmail,
// Word or Outlook is wrapped by the clipboard in <html><body> too, and
// treating those as documents would drop the user into a source view every
// time they pasted a sentence.

// The clipboard marks the part the user actually selected. Its presence means
// a live selection was copied out of a rendered page, not a file of markup.
const FRAGMENT_MARKER = /<!--\s*StartFragment\s*-->/i;

// Leading comments are skipped: a saved email opens with the tool that wrote
// it ("<!-- saved from ... -->") before its doctype, and treating that as an
// ordinary fragment would nest one document inside another.
const DOCUMENT_ROOT = /^\s*(?:<!--[\s\S]*?-->\s*)*(?:<!doctype\s+html|<html[\s>])/i;
const STYLE_BLOCK = /<style[\s>]/i;

// Tags that open a paste which is markup someone copied as text: the source of
// an email, not an email.
const SOURCE_ROOT = /^\s*<\s*(!doctype|html|head|body|style|table|div|center|td|tr)[\s>]/i;

/**
 * isDocumentBody reports whether a body is markup no editor schema can hold
 * faithfully: a whole HTML document, or one carrying its own stylesheet.
 *
 * A surface with nowhere to persist an authoring mode uses this to pick one,
 * so a designed body is never handed to the schema just because it was
 * reopened.
 */
export function isDocumentBody(html: string): boolean {
    return DOCUMENT_ROOT.test(html) || STYLE_BLOCK.test(html);
}

/**
 * detectPastedEmail returns the markup to adopt verbatim, or null when the
 * paste is ordinary content the visual editor should handle.
 */
export function detectPastedEmail(clipboard: DataTransfer | null | undefined): string | null {
    if (!clipboard) return null;

    // Markup pasted as text. Someone copying the source of an email out of a
    // file means the tags, so pasting it as literal escaped text is never what
    // they wanted.
    const text = clipboard.getData("text/plain") ?? "";
    if (SOURCE_ROOT.test(text) && /<\/[a-z][a-z0-9]*\s*>/i.test(text)) {
        return text.trim();
    }

    const html = clipboard.getData("text/html") ?? "";
    if (!html || FRAGMENT_MARKER.test(html)) return null;
    if (DOCUMENT_ROOT.test(html) || STYLE_BLOCK.test(html)) return html.trim();
    return null;
}
