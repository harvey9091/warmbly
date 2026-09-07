package replyclassify

import (
	"regexp"
	"strings"
)

// classifyLexicon is Layer 2: a deterministic, offline keyword scan over the
// subject + body. It returns (result, true) only on a CLEAR signal; ambiguous
// text returns (zero, false) so the optional model layer (or "unknown") decides.
//
// Order matters and encodes priority:
//  1. Compliance words (unsubscribe / stop / remove me / take me off) ALWAYS win.
//     Treating these as anything other than an unsubscribe request is a
//     compliance risk, so they short-circuit before sentiment.
//  2. Clear interest phrases => positive.
//  3. Clear rejection phrases => negative.
func classifyLexicon(in Input) (Result, bool) {
	text := quoteFolder.Replace(strings.ToLower(strings.TrimSpace(in.Subject + "\n" + StripQuoted(in.BodyText))))
	if text == "" {
		return Result{}, false
	}

	// 1. Compliance / opt-out (highest priority).
	if matchesOptOut(text) {
		return Result{Class: ClassUnsubscribe, Confidence: 0.9, Source: SourceLexicon}, true
	}

	// 2. Clear interest => positive.
	for _, kw := range positiveKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassPositive, Confidence: 0.8, Source: SourceLexicon}, true
		}
	}

	// 3. Clear rejection => negative.
	for _, kw := range negativeKeywords {
		if strings.Contains(text, kw) {
			return Result{Class: ClassNegative, Confidence: 0.8, Source: SourceLexicon}, true
		}
	}

	return Result{}, false
}

// unsubscribeKeywords are explicit opt-out requests. Compliance-first: any of
// these short-circuits to "unsubscribe" before sentiment is considered. Each
// is matched on word boundaries, so "stop" alone never fires on "stop by".
var unsubscribeKeywords = []string{
	"unsubscribe",
	"opt out",
	"opt-out",
	"remove me",
	"remove my email",
	"remove my address",
	"take me off",
	"stop emailing",
	"stop sending",
	"stop contacting",
	"stop these emails",
	"no more emails",
	"do not contact",
	"don't contact",
	"do not email",
	"don't email",
	"do not send",
	"don't send",
	"please stop",
	"delete my details",
	"delete my data",
	"delete my information",
}

var optOutPatterns = compileWordPatterns(unsubscribeKeywords)

// compileWordPatterns anchors each phrase on word boundaries; "don't" and
// "opt-out" keep their apostrophe and hyphen literal.
func compileWordPatterns(phrases []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(phrases))
	for _, p := range phrases {
		out = append(out, regexp.MustCompile(`(^|[^a-z0-9])`+regexp.QuoteMeta(p)+`($|[^a-z0-9])`))
	}
	return out
}

// quoteFolder folds the typographic apostrophes mail clients substitute while
// typing, so "don’t email me" matches the straight-quote phrase.
var quoteFolder = strings.NewReplacer("\u2019", "'", "\u2018", "'", "\u02bc", "'", "\u00b4", "'", "`", "'")

func matchesOptOut(lowerText string) bool {
	lowerText = quoteFolder.Replace(lowerText)
	for _, re := range optOutPatterns {
		if re.MatchString(lowerText) {
			return true
		}
	}
	return false
}

// IsOptOut reports whether a reply, read without its quoted history, asks to
// stop being emailed. It is the single check behind automatic suppression:
// the quoted original carries the sender's own opt-out line, so matching the
// whole body would opt out everyone who replies.
func IsOptOut(subject, body string) bool {
	text := strings.ToLower(strings.TrimSpace(subject + "\n" + StripQuoted(body)))
	if text == "" {
		return false
	}
	return matchesOptOut(text)
}

// quoteMarkers begin the quoted history a mail client appends to a reply.
var quoteMarkers = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^\s*on .{0,200}wrote:\s*$`),
	regexp.MustCompile(`(?im)^\s*-{2,}\s*original message\s*-{2,}\s*$`),
	regexp.MustCompile(`(?im)^\s*-{2,}\s*forwarded message\s*-{2,}\s*$`),
	regexp.MustCompile(`(?im)^\s*from:\s.+$`),
	regexp.MustCompile(`(?im)^\s*le .{0,200}a écrit\s*:\s*$`),
	regexp.MustCompile(`(?im)^\s*am .{0,200}schrieb .{0,200}:\s*$`),
}

// StripQuoted drops the quoted history from a reply body: everything from the
// first reply marker on, plus any line that is itself a ">" quote.
func StripQuoted(body string) string {
	if body == "" {
		return ""
	}
	cut := len(body)
	for _, re := range quoteMarkers {
		if loc := re.FindStringIndex(body); loc != nil && loc[0] < cut {
			cut = loc[0]
		}
	}
	body = body[:cut]
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), ">") {
			continue
		}
		kept = append(kept, ln)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// positiveKeywords are clear buying / interest signals. Kept conservative so the
// deterministic layer only fires on unambiguous intent; nuance is left to the
// model layer.
var positiveKeywords = []string{
	"interested",
	"sounds good",
	"sounds great",
	"let's chat",
	"lets chat",
	"let's talk",
	"lets talk",
	"happy to chat",
	"happy to talk",
	"set up a call",
	"book a call",
	"schedule a call",
	"schedule a demo",
	"book a demo",
	"send me more",
	"tell me more",
	"would love to",
	"count me in",
	"sign me up",
	"how much does it cost",
	"what's the pricing",
	"whats the pricing",
	"send pricing",
}

// negativeKeywords are clear rejection signals. "not interested" is the canonical
// cold-outreach brush-off.
var negativeKeywords = []string{
	"not interested",
	"no thanks",
	"no thank you",
	"not a fit",
	"not the right",
	"not relevant",
	"no need",
	"we already have",
	"we have a solution",
	"not looking",
	"wrong person",
	"wrong contact",
	"please don't",
	"leave me alone",
	"go away",
}
