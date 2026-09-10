package warmlint

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// This file turns each heuristic in Score into a location. A score with no
// location ("3 spam-trigger terms found") leaves the writer hunting through
// their own copy for words the checker already knows, so every issue points at
// the fragments that caused it and says which half of the template they are in.

// maxSpans bounds how many fragments one issue carries. The count that drives
// the deduction is computed before this cap, so trimming the list never moves
// the score.
const maxSpans = 8

// excerptMax caps a quoted line, so one pasted wall of text cannot blow up the
// response.
const excerptMax = 180

// listedTerms bounds how many trigger terms the message names before it counts
// the rest.
const listedTerms = 5

// commonField returns the field every span shares, or "" when they straddle the
// subject and the body (or there are none).
func commonField(spans []Span) string {
	field := ""
	for _, s := range spans {
		if field == "" {
			field = s.Field
			continue
		}
		if s.Field != field {
			return ""
		}
	}
	return field
}

// quotableSpans drops anything with nothing to show. A span with no text would
// render as an empty highlight, which reads as a bug rather than as a location.
func quotableSpans(spans []Span) []Span {
	out := make([]Span, 0, len(spans))
	for _, s := range spans {
		if s.Text != "" {
			out = append(out, s)
		}
	}
	return out
}

// capSpans trims the list to maxSpans, in reading order. The field is read from
// the full list before this runs: capping a subject-first list of twelve could
// otherwise drop every body span and label a both-halves issue "subject".
func capSpans(spans []Span) []Span {
	if len(spans) <= maxSpans {
		return spans
	}
	return spans[:maxSpans]
}

// spanAt describes scan[start:end] as a span. scan is what was searched and
// display is what the excerpt is quoted from: the trigger-term pass searches a
// URL-stripped copy of the text, and quoting that would show the writer a line
// with their own links blanked out. Both keep every newline, so a line number
// found in one addresses the same line in the other.
func spanAt(field, scan, display string, start, end int) Span {
	text := ""
	if start >= 0 && end <= len(scan) && start < end {
		text = strings.TrimSpace(scan[start:end])
	}
	line := lineNumber(scan, start)
	return Span{Field: field, Text: text, Line: line, Excerpt: lineText(display, line)}
}

// lineNumber is the 1-based line the byte offset sits on.
func lineNumber(s string, at int) int {
	if at < 0 || at > len(s) {
		return 0
	}
	return 1 + strings.Count(s[:at], "\n")
}

// lineText returns that line, trimmed and capped for display.
func lineText(s string, line int) string {
	if line < 1 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if line > len(lines) {
		return ""
	}
	out := strings.TrimSpace(lines[line-1])
	if r := []rune(out); len(r) > excerptMax {
		out = strings.TrimSpace(string(r[:excerptMax])) + "…"
	}
	return out
}

// scanned pairs a field with the text searched for it and the text an excerpt
// is quoted from.
type scanned struct {
	field   string
	scan    string
	display string
}

// punctuationSpans locates every run of stacked punctuation, subject first.
func punctuationSpans(subject, body string) []Span {
	var spans []Span
	for _, f := range []scanned{{FieldSubject, subject, subject}, {FieldBody, body, body}} {
		for _, loc := range stackedPunct.FindAllStringIndex(f.scan, -1) {
			spans = append(spans, spanAt(f.field, f.scan, f.display, loc[0], loc[1]))
		}
	}
	return spans
}

// termHit is one trigger term and the offset it was first seen at, in the
// folded text it was found in.
type termHit struct {
	term string
	at   int
}

// foldIndex lowercases the text and records, for every byte offset in the
// result, the offset it came from in the original.
//
// strings.ToLower maps rune by rune, and a rune can change byte length doing it
// (U+0130 is two bytes and lowercases to one, U+212A is three), so an offset
// found in the folded copy cannot be used to slice the original: past the first
// such rune it lands mid-rune and quotes bytes the writer never typed, or cuts
// one in half and produces invalid UTF-8. The map is only built when the text
// has non-ASCII in it, which is where offsets can move at all.
func foldIndex(s string) (string, []int) {
	if isASCII(s) {
		return strings.ToLower(s), nil
	}
	var b strings.Builder
	b.Grow(len(s))
	offsets := make([]int, 0, len(s)+1)
	for i, r := range s {
		before := b.Len()
		b.WriteRune(unicode.ToLower(r))
		for n := b.Len() - before; n > 0; n-- {
			offsets = append(offsets, i)
		}
	}
	// One past the end, so the end of a match at the end of the text maps too.
	offsets = append(offsets, len(s))
	return b.String(), offsets
}

// unfold translates a byte offset in the folded copy back to the original. A
// nil map means the two are byte-for-byte aligned (the ASCII path).
func unfold(offsets []int, at int) int {
	if offsets == nil {
		return at
	}
	if at < 0 || at >= len(offsets) {
		return -1
	}
	return offsets[at]
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// triggerTermsIn returns the distinct trigger terms in already-folded text, in
// order of first appearance. It matches countTriggerTerms exactly on which
// terms count; only the ordering and the offsets are extra.
func triggerTermsIn(lower string) []termHit {
	first := map[string]int{}
	for _, loc := range wordToken.FindAllStringIndex(lower, -1) {
		w := lower[loc[0]:loc[1]]
		if _, ok := triggerWords[w]; !ok {
			continue
		}
		if _, seen := first[w]; !seen {
			first[w] = loc[0]
		}
	}
	for _, p := range triggerPhrases {
		if i := strings.Index(lower, p); i >= 0 {
			if _, seen := first[p]; !seen {
				first[p] = i
			}
		}
	}
	hits := make([]termHit, 0, len(first))
	for term, at := range first {
		hits = append(hits, termHit{term: term, at: at})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].at != hits[j].at {
			return hits[i].at < hits[j].at
		}
		return hits[i].term < hits[j].term
	})
	return hits
}

// triggerSpans returns the distinct trigger terms across the template and where
// each one is. A term written in both halves counts once and is shown where it
// appears first, which keeps the count identical to countTriggerTerms over the
// two joined together.
func triggerSpans(subject, body string) (terms []string, spans []Span) {
	seen := map[string]struct{}{}
	for _, f := range []scanned{
		{FieldSubject, withoutURLs(subject), subject},
		{FieldBody, withoutURLs(body), body},
	} {
		lower, offsets := foldIndex(f.scan)
		for _, hit := range triggerTermsIn(lower) {
			if _, dup := seen[hit.term]; dup {
				continue
			}
			seen[hit.term] = struct{}{}
			terms = append(terms, hit.term)
			start, end := unfold(offsets, hit.at), unfold(offsets, hit.at+len(hit.term))
			spans = append(spans, spanAt(f.field, f.scan, f.display, start, end))
		}
	}
	return terms, spans
}

// joinTerms names the trigger terms found, counting the tail once the list gets
// long enough to read as noise.
func joinTerms(terms []string) string {
	if len(terms) <= listedTerms {
		return strings.Join(terms, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(terms[:listedTerms], ", "), len(terms)-listedTerms)
}

// linkSpans locates every anchor plus any bare URL that is not already an
// anchor's destination. Stripping tags throws hrefs away, so the text alone
// reports zero links for an HTML email; matching destinations keeps a URL used
// as its own anchor text from counting twice. The caller counts the result, so
// this returns one entry per link before any display cap.
func linkSpans(subject, body, bodyHTML string) []Span {
	destinations := map[string]struct{}{}
	var spans []Span
	for _, m := range hrefPattern.FindAllStringSubmatch(bodyHTML, -1) {
		destinations[trimURL(m[1])] = struct{}{}
		// An href lives in markup, not in a line of copy, so it carries the
		// destination and no excerpt.
		spans = append(spans, Span{Field: FieldBody, Text: m[1]})
	}
	for _, f := range []scanned{{FieldSubject, subject, subject}, {FieldBody, body, body}} {
		for _, loc := range linkPattern.FindAllStringIndex(f.scan, -1) {
			if _, seen := destinations[trimURL(f.scan[loc[0]:loc[1]])]; seen {
				continue
			}
			spans = append(spans, spanAt(f.field, f.scan, f.display, loc[0], loc[1]))
		}
	}
	return spans
}
