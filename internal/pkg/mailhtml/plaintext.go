package mailhtml

import (
	"errors"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var errNoBody = errors.New("mailhtml: parsed document has no body")

// Elements whose text is markup machinery, never words the reader sees. A
// <style> block is the important one: flattening a designed email with a naive
// tag strip put the whole stylesheet into the text/plain part.
var textSkip = map[atom.Atom]bool{
	atom.Head: true, atom.Style: true, atom.Script: true,
	atom.Title: true, atom.Noscript: true, atom.Template: true,
	atom.Iframe: true, atom.Object: true,
}

// Block elements and the number of newlines each one ends with: 2 opens a
// blank line between paragraphs, 1 just breaks the line.
var textBreak = map[atom.Atom]int{
	atom.P: 2, atom.Blockquote: 2, atom.Table: 2, atom.Ul: 2, atom.Ol: 2,
	atom.H1: 2, atom.H2: 2, atom.H3: 2, atom.H4: 2, atom.H5: 2, atom.H6: 2,
	atom.Div: 1, atom.Li: 1, atom.Tr: 1, atom.Section: 1, atom.Article: 1,
	atom.Header: 1, atom.Footer: 1, atom.Address: 1, atom.Dt: 1, atom.Dd: 1,
	atom.Pre: 2, atom.Form: 1, atom.Fieldset: 1,
}

// ToPlainText renders an HTML body down to the text/plain alternative that
// ships beside it.
//
// This is not a tag strip. A real HTML email is a table layout with a
// stylesheet, a hidden preheader and a heading structure, and flattening it
// with a regex produced one run-on line that opened with the CSS. Every inbox
// provider reads the text part, so a bad one costs deliverability as well as
// the reader who prefers text.
//
// Structure is kept: blocks break lines, list items are bulleted, table rows
// are lines and cells are spaced, and a link's destination follows its label
// so the text part is still usable. Hidden elements are dropped, because a
// preheader is written to be read once.
func ToPlainText(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return collapseText(body)
	}
	w := &textWriter{}
	root := findElement(doc, atom.Body)
	if root == nil {
		root = doc
	}
	w.walk(root, false)
	return strings.TrimSpace(w.b.String())
}

type textWriter struct {
	b strings.Builder
	// pending newlines owed before the next word, so trailing breaks never
	// reach the output and two blocks never stack four blank lines.
	pending int
	// ordered list counters, innermost last. -1 marks an unordered level.
	counters []int
	// last byte written, so a text node's leading space can be dropped when
	// one is already there rather than doubling it.
	last byte
}

func (w *textWriter) breakN(n int) {
	if w.b.Len() == 0 {
		return
	}
	if n > w.pending {
		w.pending = n
	}
}

func (w *textWriter) write(s string) {
	if s == "" {
		return
	}
	// Adjacent inline elements are separated by the whitespace between them,
	// so a text node's leading space is content unless one is already written.
	// Trimming happens before the owed newlines are flushed: a node that
	// collapses to nothing must not consume the break the next block needs.
	if w.pending > 0 || w.last == 0 || w.last == ' ' || w.last == '\n' {
		s = strings.TrimLeft(s, " ")
	}
	if s == "" {
		return
	}
	if w.pending > 0 {
		w.b.WriteString(strings.Repeat("\n", min(w.pending, 2)))
		w.pending = 0
	}
	w.b.WriteString(s)
	w.last = s[len(s)-1]
}

func (w *textWriter) walk(n *html.Node, pre bool) {
	switch n.Type {
	case html.TextNode:
		if pre {
			w.write(n.Data)
			return
		}
		w.write(collapseText(n.Data))
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.walk(c, pre)
		}
		return
	}

	if textSkip[n.DataAtom] || isHidden(n) {
		return
	}

	switch n.DataAtom {
	case atom.Br:
		w.breakN(1)
		return
	case atom.Hr:
		w.breakN(2)
		w.write("---")
		w.breakN(2)
		return
	case atom.Img:
		// An alt is the only thing an image contributes to a text part, and a
		// spacer or a tracking pixel has none.
		if alt := strings.TrimSpace(attrOf(n, "alt")); alt != "" {
			w.write("[" + alt + "]")
		}
		return
	case atom.A:
		w.writeAnchor(n, pre)
		return
	case atom.Td, atom.Th:
		// Cells are columns of a layout table, not lines of their own.
		if w.b.Len() > 0 && w.pending == 0 {
			w.write(" ")
		}
	case atom.Ol, atom.Ul:
		start := 0
		if n.DataAtom == atom.Ul {
			start = -1
		}
		w.counters = append(w.counters, start)
		defer func() { w.counters = w.counters[:len(w.counters)-1] }()
	case atom.Li:
		w.breakN(1)
		w.write(w.bullet())
	}

	if br := textBreak[n.DataAtom]; br > 0 && n.DataAtom != atom.Li {
		w.breakN(br)
	}
	inPre := pre || n.DataAtom == atom.Pre
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c, inPre)
	}
	if br := textBreak[n.DataAtom]; br > 0 {
		w.breakN(br)
	}
}

// bullet numbers an <ol> item and dashes a <ul> one, tracking the innermost
// list so a <ul> nested in an <ol> does not inherit its numbering.
func (w *textWriter) bullet() string {
	if len(w.counters) == 0 || w.counters[len(w.counters)-1] < 0 {
		return "- "
	}
	w.counters[len(w.counters)-1]++
	return strconv.Itoa(w.counters[len(w.counters)-1]) + ". "
}

// writeAnchor keeps a link usable in text: the label, then the destination in
// parentheses unless the label already is the destination.
func (w *textWriter) writeAnchor(n *html.Node, pre bool) {
	inner := &textWriter{counters: w.counters}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		inner.walk(c, pre)
	}
	label := strings.TrimSpace(inner.b.String())
	w.write(label)

	href := strings.TrimSpace(attrOf(n, "href"))
	if href == "" || strings.HasPrefix(href, "#") {
		return
	}
	scheme := strings.ToLower(href)
	if !strings.HasPrefix(scheme, "http://") && !strings.HasPrefix(scheme, "https://") && !strings.HasPrefix(scheme, "mailto:") {
		return
	}
	if label == "" {
		w.write(href)
		return
	}
	if label == href || "mailto:"+label == href {
		return
	}
	w.write(" (" + href + ")")
}

// isHidden reports whether an element is styled out of view. A preheader is
// hidden text written to show in the inbox list only, and repeating it as the
// first line of the text part is exactly what it was written to avoid.
func isHidden(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "hidden" {
			return true
		}
	}
	style := strings.ToLower(attrOf(n, "style"))
	if style == "" {
		return false
	}
	for _, d := range parseDeclarations(style) {
		if d.prop == "display" && strings.HasPrefix(d.value, "none") {
			return true
		}
	}
	return false
}

// collapseText folds every run of whitespace to one space, the way a browser
// lays out an HTML text node. A leading or trailing space is kept: it is the
// gap between two inline elements, and dropping it ran their words together.
func collapseText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\v', '\u00a0':
			space = true
		default:
			if space {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}
