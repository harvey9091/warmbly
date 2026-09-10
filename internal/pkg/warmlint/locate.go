package warmlint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/warmbly/warmbly/internal/pkg/casefold"
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

// termHit is one occurrence of one trigger term, at an offset in the folded
// text it was found in.
type termHit struct {
	term string
	at   int
}

// triggerTermsIn returns EVERY occurrence of every trigger term in already-
// folded text, in reading order. Which terms count is identical to
// countTriggerTerms; the offsets and the repeats are extra, and the caller
// deduplicates for scoring.
func triggerTermsIn(lower string) []termHit {
	var hits []termHit
	for _, loc := range wordToken.FindAllStringIndex(lower, -1) {
		w := lower[loc[0]:loc[1]]
		if _, ok := triggerWords[w]; ok {
			hits = append(hits, termHit{term: w, at: loc[0]})
		}
	}
	for _, p := range triggerPhrases {
		for from := 0; from < len(lower); {
			i := strings.Index(lower[from:], p)
			if i < 0 {
				break
			}
			hits = append(hits, termHit{term: p, at: from + i})
			from += i + len(p)
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].at != hits[j].at {
			return hits[i].at < hits[j].at
		}
		return hits[i].term < hits[j].term
	})
	return hits
}

// occurrence is one place a term was written, already resolved to a span.
type occurrence struct {
	key  string
	span Span
}

// triggerSpans returns the distinct trigger terms across the template and every
// place each one is written.
//
// Two things are deliberate here. A term counts ONCE towards the score however
// often it appears, which keeps the count identical to countTriggerTerms over
// the two halves joined together, while every occurrence still gets a span:
// pointing at one "free" out of three sends the writer back to hunt for the
// other two on the next re-check.
//
// And the spans come out round by round rather than in reading order, so every
// term shows once in every half it appears in before any term shows twice. The
// list is capped for display, and a word written twenty times would otherwise
// fill it and hide every other term that is also wrong.
func triggerSpans(subject, body string) (terms []string, spans []Span) {
	counted := map[string]struct{}{}
	rounds := map[string][]Span{}
	var order []string
	for _, f := range []scanned{
		{FieldSubject, withoutURLs(subject), subject},
		{FieldBody, withoutURLs(body), body},
	} {
		lower, offsets := casefold.Index(f.scan)
		for _, hit := range triggerTermsIn(lower) {
			if _, dup := counted[hit.term]; !dup {
				counted[hit.term] = struct{}{}
				terms = append(terms, hit.term)
			}
			start, end := casefold.Origin(offsets, hit.at), casefold.Origin(offsets, hit.at+len(hit.term))
			key := f.field + "\x00" + hit.term
			if _, seen := rounds[key]; !seen {
				order = append(order, key)
			}
			rounds[key] = append(rounds[key], spanAt(f.field, f.scan, f.display, start, end))
		}
	}
	for r := 0; ; r++ {
		emitted := false
		for _, key := range order {
			if r < len(rounds[key]) {
				spans = append(spans, rounds[key][r])
				emitted = true
			}
		}
		if !emitted {
			break
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
	// Destinations first and separately: a bare URL only counts when it is not
	// already an anchor's target, and that has to be known before the subject
	// is scanned.
	destinations := map[string]struct{}{}
	anchors := make([]Span, 0)
	for _, m := range hrefPattern.FindAllStringSubmatch(bodyHTML, -1) {
		destinations[trimURL(m[1])] = struct{}{}
		// An href lives in markup, not in a line of copy, so it carries the
		// destination and no excerpt.
		anchors = append(anchors, Span{Field: FieldBody, Text: m[1]})
	}

	bare := func(f scanned) []Span {
		var out []Span
		for _, loc := range linkPattern.FindAllStringIndex(f.scan, -1) {
			if _, seen := destinations[trimURL(f.scan[loc[0]:loc[1]])]; seen {
				continue
			}
			out = append(out, spanAt(f.field, f.scan, f.display, loc[0], loc[1]))
		}
		return out
	}

	// Subject first, like every other span list: a body with more anchors than
	// the display cap would otherwise push the subject's own link off the end
	// while the issue still claims to cover both halves.
	spans := bare(scanned{FieldSubject, subject, subject})
	spans = append(spans, anchors...)
	return append(spans, bare(scanned{FieldBody, body, body})...)
}
