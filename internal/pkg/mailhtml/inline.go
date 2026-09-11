package mailhtml

import (
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// maxInlineBytes bounds the work the inliner will take on. A body past this is
// already unsendable (Gmail clips at ~102 KB), so it ships as written rather
// than paying for a parse nobody benefits from.
const maxInlineBytes = 2 << 20

// Elements a declaration can never usefully land on.
var noInlineTarget = map[string]bool{
	"html": true, "head": true, "meta": true, "title": true,
	"link": true, "base": true, "script": true, "style": true,
}

// Pseudo-classes and pseudo-elements whose match depends on reader state, so
// their rule has to stay in the <style> block instead of being inlined.
var statefulPseudo = []string{
	":hover", ":active", ":focus", ":focus-within", ":focus-visible",
	":visited", ":link", ":target", ":checked", ":disabled", ":enabled",
	"::",
}

// HasStyleBlock reports whether a body carries an embedded stylesheet, which
// is the only case InlineCSS is not a no-op.
func HasStyleBlock(body string) bool {
	return styleOpen.MatchString(body)
}

// InlineCSS moves every rule of a body's <style> blocks onto the elements it
// matches, as a style attribute.
//
// Outlook.com, Yahoo and Gmail's mobile clients drop or rewrite embedded
// stylesheets, so a design written with classes arrives unstyled. Inlining is
// what makes "HTML and CSS" mean the same thing in every inbox.
//
// A <style data-warmbly-inline="false"> block is left exactly as written, for
// an author who knows their readers and wants the cascade they wrote.
//
// Rules that cannot be inlined stay behind: at-rules (@media, @font-face,
// @keyframes, @supports, @import) have no element to attach to, and a rule
// keyed on reader state (:hover and friends) has no value at send time. The
// cascade is honoured, so an author's own style attribute still wins over a
// sheet rule of any specificity, and !important still beats it.
//
// The body is returned unchanged when it has no stylesheet, when it is too
// large to be worth parsing, or when anything about the parse goes wrong: a
// message that ships as the author wrote it is always better than one this
// pass mangled.
func InlineCSS(body string) string {
	if body == "" || len(body) > maxInlineBytes || !HasStyleBlock(body) {
		return body
	}

	fragment := !isFullDocument(body)
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body
	}

	sheets := collectStyleNodes(doc)
	if len(sheets) == 0 {
		return body
	}

	// One pass over every stylesheet, in document order, so the later rule of
	// two with equal specificity wins as it does in a browser.
	pending := map[*html.Node]map[string]staged{}
	order := 0
	for _, sheet := range sheets {
		if !sheetEligible(sheet) {
			continue
		}
		items := parseStylesheet(textOf(sheet))
		// Set once a rule has actually moved onto an element. Until then the
		// sheet is left byte for byte as written: rewriting it from what the
		// parser understood would delete anything it did not, which for CSS
		// we cannot read is silent loss of the author's design.
		movedAny := false
		var kept []string
		for _, item := range items {
			if item.verbatim != "" {
				kept = append(kept, item.verbatim)
				continue
			}
			// A selector list is split on commas, which is only safe once
			// every piece parses: a comma inside :is(...) or an attribute
			// value belongs to the selector, and rejoining the pieces would
			// write a different one. If any piece fails, the whole rule is
			// left alone with its selectors exactly as the author wrote them.
			pieces := splitSelectorList(item.selectors)
			compiled := make([]cascadia.Sel, len(pieces))
			readable := len(pieces) > 0
			for i, sel := range pieces {
				c, perr := cascadia.Parse(sel)
				if perr != nil {
					readable = false
					break
				}
				compiled[i] = c
			}
			if !readable {
				kept = append(kept, item.selectors+" {"+declString(item.decls)+"}")
				continue
			}

			// Selectors this pass could not take over: a stateful one, or one
			// that matched nothing. They keep the rule, restricted to
			// themselves, because a list like ".btn, .other:hover" inlines its
			// first half and would otherwise take the declaration the second
			// half still needs with it.
			var unhandled []string
			for i, sel := range pieces {
				if isStateful(sel) {
					unhandled = append(unhandled, sel)
					continue
				}
				matched := false
				for _, node := range cascadia.QueryAll(doc, compiled[i]) {
					if noInlineTarget[node.Data] {
						continue
					}
					stage(pending, node, item.decls, compiled[i].Specificity(), order)
					matched = true
					movedAny = true
				}
				if !matched {
					unhandled = append(unhandled, sel)
				}
				order++
			}
			switch {
			case len(unhandled) == len(pieces):
				// Nothing moved, so the author's own text is kept.
				kept = append(kept, item.selectors+" {"+declString(item.decls)+"}")
			case len(unhandled) > 0:
				kept = append(kept, strings.Join(unhandled, ", ")+" {"+declString(item.decls)+"}")
			}
		}
		if movedAny {
			setText(sheet, strings.Join(kept, "\n"))
		}
	}

	applyStaged(doc, pending)
	dropEmptyStyles(sheets)

	out, rerr := renderTo(doc, fragment)
	if rerr != nil {
		return body
	}
	// A last net under the whole pass: whatever went wrong, a message that
	// ships as the author wrote it beats one this pass emptied.
	if strings.TrimSpace(out) == "" && strings.TrimSpace(body) != "" {
		return body
	}
	return out
}

// sheetEligible reports whether a <style> block is one this pass will take
// rules out of. A print or device-scoped sheet is not what the reader sees,
// and data-warmbly-inline="false" is the author's opt out, per stylesheet
// rather than per campaign. Shared with Lint so the editor never promises
// inlining for a sheet that will ship exactly as written.
func sheetEligible(sheet *html.Node) bool {
	return mediaAttr(sheet) == "" && !strings.EqualFold(attrOf(sheet, "data-warmbly-inline"), "false")
}

// staged is the winning declaration for one property on one element, with the
// cascade key that let it win.
type staged struct {
	decl  cssDecl
	rank  int // 0 sheet, 1 inline, 2 sheet !important, 3 inline !important
	spec  cascadia.Specificity
	order int
}

// stage records a rule's declarations against one element. Staging is kept in
// a caller-owned map, never a package global: sends run concurrently.
func stage(pending map[*html.Node]map[string]staged, node *html.Node, decls []cssDecl, spec cascadia.Specificity, order int) {
	bucket := pending[node]
	if bucket == nil {
		bucket = map[string]staged{}
		pending[node] = bucket
	}
	for i, d := range decls {
		rank := 0
		if d.important {
			rank = 2
		}
		cand := staged{decl: d, rank: rank, spec: spec, order: order*1000 + i}
		if cur, ok := bucket[d.prop]; ok && !beats(cand, cur) {
			continue
		}
		bucket[d.prop] = cand
	}
}

// beats applies the cascade: importance first, then specificity, then the
// later declaration.
func beats(a, b staged) bool {
	if a.rank != b.rank {
		return a.rank > b.rank
	}
	if a.spec != b.spec {
		return b.spec.Less(a.spec)
	}
	return a.order >= b.order
}

// applyStaged merges each element's staged declarations with the style
// attribute its author wrote and writes the result back.
func applyStaged(root *html.Node, pending map[*html.Node]map[string]staged) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if bucket, ok := pending[n]; ok {
				writeStyle(n, bucket)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
}

func writeStyle(n *html.Node, bucket map[string]staged) {
	own := parseDeclarations(attrOf(n, "style"))
	for i, d := range own {
		rank := 1
		if d.important {
			rank = 3
		}
		// An author's inline declaration outranks any selector, so it needs a
		// specificity no sheet rule can reach.
		cand := staged{decl: d, rank: rank, spec: cascadia.Specificity{1 << 20, 0, 0}, order: 1<<30 + i}
		if cur, ok := bucket[d.prop]; ok && !beats(cand, cur) {
			continue
		}
		bucket[d.prop] = cand
	}

	winners := make([]staged, 0, len(bucket))
	for _, s := range bucket {
		winners = append(winners, s)
	}
	// Source order, sheet before inline: a shorthand has to precede the
	// longhand that narrows it, or "margin:0; margin-top:8px" loses its top.
	sort.Slice(winners, func(i, j int) bool { return winners[i].order < winners[j].order })

	decls := make([]cssDecl, 0, len(winners))
	for _, w := range winners {
		decls = append(decls, w.decl)
	}
	setAttr(n, "style", declString(decls))
}

// isStateful reports whether a selector depends on something only the reader's
// client knows, which no send-time pass can resolve.
func isStateful(sel string) bool {
	lower := strings.ToLower(sel)
	for _, p := range statefulPseudo {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func collectStyleNodes(root *html.Node) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Style {
			found = append(found, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

// dropEmptyStyles removes the <style> elements every rule was inlined out of,
// so the payload does not carry the stylesheet twice. Gmail clips a message
// past ~102 KB, and inlining only adds bytes.
func dropEmptyStyles(sheets []*html.Node) {
	for _, s := range sheets {
		if strings.TrimSpace(textOf(s)) != "" || s.Parent == nil {
			continue
		}
		s.Parent.RemoveChild(s)
	}
}

// splitSelectorList breaks "a, b" into its selectors, ignoring commas inside
// parentheses, brackets and strings, where they belong to the selector rather
// than separating two.
func splitSelectorList(list string) []string {
	var out []string
	depth := 0
	quote := byte(0)
	start := 0
	for i := 0; i < len(list); i++ {
		c := list[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		case c == ',' && depth == 0:
			if sel := strings.TrimSpace(list[start:i]); sel != "" {
				out = append(out, sel)
			}
			start = i + 1
		}
	}
	if sel := strings.TrimSpace(list[start:]); sel != "" {
		out = append(out, sel)
	}
	return out
}
