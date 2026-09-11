package mailhtml

import "strings"

// A stylesheet is split into two kinds of item. A plain rule has a selector
// list and can be moved onto the elements it matches; an at-rule (@media,
// @font-face, @keyframes, @supports, @import) has no single element to attach
// to and can only stay in the <style> block.
type cssItem struct {
	// verbatim holds text that is carried through exactly as written: an
	// at-rule, or a stretch the parser could not read. Empty for a plain rule.
	//
	// Unreadable text has to become an item rather than being dropped. The
	// sheet is rewritten from these items as soon as one rule moves onto an
	// element, so anything not represented here is deleted from the author's
	// stylesheet the moment any other rule inlines.
	verbatim string
	// selectors is the raw selector list of a plain rule ("h1, .lead > p").
	selectors string
	decls     []cssDecl
}

type cssDecl struct {
	prop      string
	value     string
	important bool
}

// parseStylesheet splits CSS into its top-level items. It is deliberately
// forgiving: mail templates carry hand-written and vendor-mangled CSS, and a
// rule we cannot read is dropped rather than failing the whole sheet.
func parseStylesheet(css string) []cssItem {
	var items []cssItem
	for i := 0; i < len(css); {
		i = skipCSSFiller(css, i)
		if i >= len(css) {
			break
		}
		if css[i] == '@' {
			start := i
			prelude := scanCSSUntil(css, i, "{;")
			if prelude >= len(css) {
				items = appendVerbatim(items, css[start:])
				break
			}
			if css[prelude] == ';' {
				items = append(items, cssItem{verbatim: strings.TrimSpace(css[start : prelude+1])})
				i = prelude + 1
				continue
			}
			end := scanCSSBlock(css, prelude)
			items = append(items, cssItem{verbatim: strings.TrimSpace(css[start:end])})
			i = end
			continue
		}
		selEnd := scanCSSUntil(css, i, "{")
		if selEnd >= len(css) {
			items = appendVerbatim(items, css[i:])
			break
		}
		blockEnd := scanCSSBlock(css, selEnd)
		selectors := strings.TrimSpace(css[i:selEnd])
		// blockEnd sits one past the closing brace; an unterminated block runs
		// to the end of the sheet and has no brace to trim.
		innerEnd := blockEnd
		if innerEnd > selEnd+1 && css[innerEnd-1] == '}' {
			innerEnd--
		}
		inner := css[selEnd+1 : innerEnd]
		decls := parseDeclarations(inner)
		switch {
		case selectors != "" && len(decls) > 0:
			items = append(items, cssItem{selectors: selectors, decls: decls})
		default:
			// A rule with no selector, or one whose declarations we could not
			// read: kept as written rather than dropped.
			items = appendVerbatim(items, css[i:blockEnd])
		}
		i = blockEnd
	}
	return items
}

// appendVerbatim adds a stretch of unreadable stylesheet text, ignoring one
// that is only whitespace.
func appendVerbatim(items []cssItem, text string) []cssItem {
	if t := strings.TrimSpace(text); t != "" {
		items = append(items, cssItem{verbatim: t})
	}
	return items
}

// parseDeclarations reads "prop: value; prop: value" into ordered pairs.
func parseDeclarations(block string) []cssDecl {
	var decls []cssDecl
	for i := 0; i < len(block); {
		end := scanCSSUntil(block, i, ";")
		chunk := block[i:min(end, len(block))]
		i = end + 1

		colon := -1
		for j := 0; j < len(chunk); j++ {
			j = skipCSSToken(chunk, j)
			if j < len(chunk) && chunk[j] == ':' {
				colon = j
				break
			}
		}
		if colon <= 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(chunk[:colon]))
		value := strings.TrimSpace(chunk[colon+1:])
		if prop == "" || value == "" {
			continue
		}
		// Matched on the value itself, never on a lowercased copy: ToLower can
		// change a string's length, so an index taken from one does not point
		// at the same place in the other (see hasPrefixFold in document.go).
		important := false
		if idx := lastIndexFold(value, "!important"); idx >= 0 && strings.TrimSpace(value[idx+len("!important"):]) == "" {
			important = true
			value = strings.TrimSpace(value[:idx])
		}
		if value == "" {
			continue
		}
		decls = append(decls, cssDecl{prop: prop, value: value, important: important})
	}
	return decls
}

// skipCSSFiller advances past whitespace and comments.
func skipCSSFiller(css string, i int) int {
	for i < len(css) {
		if css[i] == ' ' || css[i] == '\t' || css[i] == '\n' || css[i] == '\r' || css[i] == '\f' {
			i++
			continue
		}
		if strings.HasPrefix(css[i:], "/*") {
			if end := strings.Index(css[i+2:], "*/"); end >= 0 {
				i += 2 + end + 2
				continue
			}
			return len(css)
		}
		return i
	}
	return i
}

// skipCSSToken advances past one string, comment or parenthesised group at i,
// so a delimiter search never stops inside url(a;b) or "a{b}". It returns the
// index of that token's last character, or i unchanged for an ordinary one.
func skipCSSToken(css string, i int) int {
	switch {
	case strings.HasPrefix(css[i:], "/*"):
		if end := strings.Index(css[i+2:], "*/"); end >= 0 {
			return i + 2 + end + 1
		}
		return len(css)
	case css[i] == '"' || css[i] == '\'':
		return skipCSSQuoted(css, i)
	case css[i] == '(':
		depth := 1
		for j := i + 1; j < len(css); j++ {
			switch {
			case strings.HasPrefix(css[j:], "/*"):
				end := strings.Index(css[j+2:], "*/")
				if end < 0 {
					return len(css)
				}
				j += 2 + end + 1
			case css[j] == '"' || css[j] == '\'':
				j = skipCSSQuoted(css, j)
			case css[j] == '(':
				depth++
			case css[j] == ')':
				depth--
				if depth == 0 {
					return j
				}
			}
		}
		return len(css)
	}
	return i
}

// skipCSSQuoted returns the index of the quote closing the string that opens
// at i, or the end of the input for an unterminated one.
func skipCSSQuoted(css string, i int) int {
	quote := css[i]
	for j := i + 1; j < len(css); j++ {
		if css[j] == '\\' {
			j++
			continue
		}
		if css[j] == quote {
			return j
		}
	}
	return len(css)
}

// scanCSSUntil returns the index of the first character in stop that sits at
// the top level, or len(css) when there is none.
func scanCSSUntil(css string, i int, stop string) int {
	for ; i < len(css); i++ {
		i = skipCSSToken(css, i)
		if i >= len(css) {
			break
		}
		if strings.IndexByte(stop, css[i]) >= 0 {
			return i
		}
	}
	return len(css)
}

// scanCSSBlock takes the index of a '{' and returns the index one past its
// matching '}'. An unterminated block runs to the end of the sheet.
func scanCSSBlock(css string, open int) int {
	depth := 0
	for i := open; i < len(css); i++ {
		i = skipCSSToken(css, i)
		if i >= len(css) {
			break
		}
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(css)
}

// declString renders declarations back to the text of a style attribute.
func declString(decls []cssDecl) string {
	var b strings.Builder
	for _, d := range decls {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d.prop)
		b.WriteString(": ")
		b.WriteString(d.value)
		if d.important {
			b.WriteString(" !important")
		}
	}
	return b.String()
}

// lastIndexFold is strings.LastIndex with ASCII case folding.
func lastIndexFold(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if hasPrefixFold(s[i:], sub) {
			return i
		}
	}
	return -1
}
