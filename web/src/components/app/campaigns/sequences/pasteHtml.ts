// Paste normaliser for the campaign body editor (issue #380).
//
// Copying a message out of Gmail, Outlook or Word brings its own idea of a
// blank line with it: Gmail writes one as `<div><br></div>`, Word as an empty
// `<p class=MsoNormal>` holding `&nbsp;`. Our paragraphs already carry their
// own bottom margin, so pasting those verbatim renders every gap twice — the
// reported double spacing. This strips the source's spacing scaffolding (and
// its editor cruft) and leaves the structure: paragraphs, breaks, lists,
// headings, links, images and the inline marks the schema keeps.

// Elements whose content must not survive a paste: a <style> block's CSS is
// text to the parser and would land in the body as copy.
const DROP_WITH_CONTENT = new Set(["STYLE", "SCRIPT", "META", "LINK", "TITLE", "HEAD", "NOSCRIPT"]);

// Blocks that render their own gap in our editor, so an empty one is spacing
// the source drew by hand and we draw with CSS.
const SPACING_BLOCKS = new Set(["P", "DIV", "H1", "H2", "H3", "H4", "H5", "H6"]);

// An image src a mail client can actually load. A `cid:` part belongs to the
// message it was copied from and a `data:` blob is stripped by most clients, so
// both would only ever render as a broken image for the recipient.
function isLoadableImage(src: string): boolean {
    return /^https?:\/\//i.test(src.trim());
}

// isBlankBlock reports whether a block holds nothing but whitespace, <br> and
// non-breaking spaces — the shapes every mail client writes a blank line as.
function isBlankBlock(el: Element): boolean {
    if (el.querySelector("img, table, hr, iframe, video")) return false;
    // U+00A0 is the &nbsp; Word fills its spacer paragraphs with.
    return (el.textContent ?? "").replace(/[\s\u00a0]+/g, "") === "";
}

// stripTrailingBreaks drops the <br>s a block ends with. Gmail closes a line
// with one, and it renders as an extra blank line inside a paragraph that
// already has a margin under it.
function stripTrailingBreaks(el: Element) {
    let last = el.lastChild;
    while (last) {
        if (last.nodeType === Node.TEXT_NODE && (last.textContent ?? "").replace(/[\s\u00a0]+/g, "") === "") {
            const prev = last.previousSibling;
            last.parentNode?.removeChild(last);
            last = prev;
            continue;
        }
        if (last.nodeType === Node.ELEMENT_NODE && (last as Element).tagName === "BR") {
            const prev = last.previousSibling;
            last.parentNode?.removeChild(last);
            last = prev;
            continue;
        }
        break;
    }
}

// collapseBreakRuns turns a run of consecutive <br>s into one. Two in a row is
// how a mail client writes a paragraph break inside a block; keeping both would
// stack an empty line on top of the margin our paragraphs already have.
function collapseBreakRuns(root: ParentNode) {
    for (const br of Array.from(root.querySelectorAll("br"))) {
        let next = br.nextSibling;
        while (next) {
            if (next.nodeType === Node.TEXT_NODE && (next.textContent ?? "").replace(/[\s\u00a0]+/g, "") === "") {
                const after = next.nextSibling;
                next.parentNode?.removeChild(next);
                next = after;
                continue;
            }
            if (next.nodeType === Node.ELEMENT_NODE && (next as Element).tagName === "BR") {
                const after = next.nextSibling;
                next.parentNode?.removeChild(next);
                next = after;
                continue;
            }
            break;
        }
    }
}

// unwrap replaces an element with its children, keeping the content.
function unwrap(el: Element) {
    const parent = el.parentNode;
    if (!parent) return;
    while (el.firstChild) parent.insertBefore(el.firstChild, el);
    parent.removeChild(el);
}

// Chip spans our own nodes serialize to. A copy from one step's body into
// another arrives as ordinary HTML, so these have to survive the span unwrap or
// the merge fields, AI blocks, conditions and form links land as literal text.
const KEEP_SPAN_ATTRS = ["data-var", "data-ai-var", "data-if", "data-form-link"];

export function normalizePastedHTML(html: string): string {
    if (!html || typeof window === "undefined" || typeof DOMParser === "undefined") return html;
    // A copy from inside a TipTap editor is already a document in our own
    // schema; ProseMirror marks it and re-parses it exactly. Nothing to fix.
    if (html.includes("data-pm-slice")) return html;

    const doc = new DOMParser().parseFromString(html, "text/html");
    const body = doc.body;
    if (!body) return html;

    // Comments carry Word's conditional markup, which is a second copy of the
    // document the parser would otherwise leave lying in the output.
    const walker = doc.createTreeWalker(body, NodeFilter.SHOW_COMMENT);
    const comments: Comment[] = [];
    while (walker.nextNode()) comments.push(walker.currentNode as Comment);
    for (const c of comments) c.parentNode?.removeChild(c);

    for (const el of Array.from(body.querySelectorAll("*"))) {
        if (!el.isConnected) continue;
        const tag = el.tagName;
        if (DROP_WITH_CONTENT.has(tag)) {
            el.remove();
            continue;
        }
        // Word's namespaced elements (<o:p>, <w:sdt>) hold nothing our editor
        // can use but do hold the &nbsp; that makes a spacer paragraph look
        // non-empty, so they go with their content.
        if (tag.includes(":")) {
            el.remove();
            continue;
        }
        if (tag === "IMG") {
            const src = el.getAttribute("src") ?? "";
            if (!isLoadableImage(src)) el.remove();
            continue;
        }
        // A <font> or <span> carries only styling our schema drops anyway;
        // unwrapping keeps the text and loses the wrapper.
        if (tag === "FONT" || tag === "SPAN" || tag === "CENTER") {
            if (KEEP_SPAN_ATTRS.some((a) => el.hasAttribute(a))) continue;
            unwrap(el);
        }
    }

    collapseBreakRuns(body);

    // Blank spacing blocks, innermost first, so a wrapper that only held one
    // becomes blank in turn and goes with it.
    for (const el of Array.from(body.querySelectorAll("p, div, h1, h2, h3, h4, h5, h6")).reverse()) {
        if (!el.isConnected) continue;
        if (SPACING_BLOCKS.has(el.tagName) && isBlankBlock(el)) el.remove();
    }

    for (const el of Array.from(body.querySelectorAll("p, div, li, h1, h2, h3, h4, h5, h6, td"))) {
        stripTrailingBreaks(el);
    }

    return body.innerHTML;
}
