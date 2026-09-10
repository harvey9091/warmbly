package warmlint

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func hasIssue(res ScoreResult, code string) bool {
	for _, i := range res.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestScoreCleanCopyIsUnpenalized(t *testing.T) {
	res := Score(
		"Quick question about your hiring process",
		"<p>Hi Ada, saw you are growing the support team. Worth a chat?</p>",
		"Hi Ada, saw you are growing the support team. Worth a chat?",
	)
	if res.Score != 100 {
		t.Errorf("clean copy scored %d with %+v, want 100", res.Score, res.Issues)
	}
}

func TestScoreImageHeavyBody(t *testing.T) {
	// Almost all image, no readable text: filters cannot read it, and that is
	// treated as evasion rather than as a design choice.
	res := Score("Hello", `<img src="a.png"><img src="b.png">`, "")
	if !hasIssue(res, "image_heavy") {
		t.Errorf("image-only body not flagged: %+v", res.Issues)
	}

	// Plenty of text, a few images: fine.
	body := strings.Repeat("A real sentence about the recipient's work. ", 20)
	res = Score("Hello", "<p>"+body+`</p><img src="a.png">`, body)
	if hasIssue(res, "image_heavy") || hasIssue(res, "many_images") {
		t.Errorf("ordinary copy with one image was flagged: %+v", res.Issues)
	}

	// Many images alongside text is a softer warning.
	res = Score("Hello", "<p>"+body+`</p>`+strings.Repeat(`<img src="x.png">`, 6), body)
	if !hasIssue(res, "many_images") {
		t.Errorf("six images not flagged: %+v", res.Issues)
	}
	if hasIssue(res, "image_heavy") {
		t.Errorf("text-rich body wrongly flagged as image-only: %+v", res.Issues)
	}
}

func TestScoreWithAttachments(t *testing.T) {
	subject, html, plain := "Following up", "<p>Hi Ada, following up on my note.</p>", "Hi Ada, following up on my note."

	base := ScoreWithAttachments(subject, html, plain, 0)
	if hasIssue(base, "has_attachments") {
		t.Errorf("no attachments should not flag: %+v", base.Issues)
	}

	withAtt := ScoreWithAttachments(subject, html, plain, 2)
	if !hasIssue(withAtt, "has_attachments") {
		t.Errorf("attachments not flagged: %+v", withAtt.Issues)
	}
	if withAtt.Score >= base.Score {
		t.Errorf("attachment score %d not below the clean %d", withAtt.Score, base.Score)
	}
}

func TestScoreNeverGoesNegative(t *testing.T) {
	// Every heuristic at once: the floor is 0, not a negative number the UI
	// would have to special-case.
	res := ScoreWithAttachments(
		"FREE CASH PRIZE GUARANTEED!!!",
		`<img src="a.png"><img src="b.png"><img src="c.png"><img src="d.png">`,
		"",
		5,
	)
	if res.Score < 0 {
		t.Errorf("score = %d, want a floor of 0", res.Score)
	}
	if len(res.Issues) == 0 {
		t.Error("obviously spammy copy produced no issues")
	}
}

func TestScoreCountsLinksInHTMLAnchors(t *testing.T) {
	// A normal HTML email carries its URLs in href attributes. Stripping tags
	// throws those away, so counting the text alone reported zero links and the
	// cap never fired for the case it exists for.
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	html := "<p>" + body + "</p><p>" +
		strings.Repeat(`<a href="https://example.com/x">see this</a> `, 6) + "</p>"

	res := Score("Quick question", html, "")
	if !hasIssue(res, "too_many_links") {
		t.Errorf("six anchors in an HTML body were not counted: %+v", res.Issues)
	}

	// The editor stores a plain-text body derived from the HTML, which also
	// drops the hrefs. The score must not depend on which one is present.
	res = Score("Quick question", html, body)
	if !hasIssue(res, "too_many_links") {
		t.Errorf("six anchors alongside a link-free plain body were not counted: %+v", res.Issues)
	}
}

func TestScoreDoesNotDoubleCountSelfLinkingAnchors(t *testing.T) {
	// A URL used as its own anchor text appears in both the href and the text.
	// Counting both would flag three links as six.
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	html := "<p>" + body + "</p><p>" +
		`<a href="https://a.com/1">https://a.com/1</a> ` +
		`<a href="https://b.com/2">https://b.com/2</a> ` +
		`<a href="https://c.com/3">https://c.com/3</a>` + "</p>"
	plain := body + " https://a.com/1 https://b.com/2 https://c.com/3"

	res := Score("Quick question", html, plain)
	if hasIssue(res, "too_many_links") {
		t.Errorf("three self-linking anchors were counted as more: %+v", res.Issues)
	}
}

func TestScoreStillCountsPlainTextURLs(t *testing.T) {
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	plain := body + " https://a.com/1 https://b.com/2 https://c.com/3 https://d.com/4 https://e.com/5"

	res := Score("Quick question", "", plain)
	if !hasIssue(res, "too_many_links") {
		t.Errorf("five bare URLs in a plain body were not counted: %+v", res.Issues)
	}
}

func TestScoreCountsAnchorsAndBareURLsTogether(t *testing.T) {
	// Labeled anchors and bare URLs are different destinations. Counting only
	// the larger of the two sets let six distinct links score a clean 100.
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	html := "<p>" + body + "</p><p>" +
		`<a href="https://a.com/1">one</a> <a href="https://b.com/2">two</a> <a href="https://c.com/3">three</a> ` +
		"https://d.com/4 https://e.com/5 https://f.com/6</p>"
	plain := body + " one two three https://d.com/4 https://e.com/5 https://f.com/6"

	res := Score("Quick question", html, plain)
	if !hasIssue(res, "too_many_links") {
		t.Errorf("three anchors plus three bare URLs were not counted as six: %+v", res.Issues)
	}
}

func TestScoreIgnoresTrailingPunctuationWhenMatchingAnchors(t *testing.T) {
	// A URL that ends a sentence in the plain text is the same link as the
	// anchor's destination, so it must not count a second time.
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	html := "<p>" + body + "</p><p>" +
		`<a href="https://a.com/1">https://a.com/1</a>, <a href="https://b.com/2">https://b.com/2</a>.` + "</p>"
	plain := body + " https://a.com/1, https://b.com/2."

	res := Score("Quick question", html, plain)
	if hasIssue(res, "too_many_links") {
		t.Errorf("two self-linking anchors counted as more than two: %+v", res.Issues)
	}
}

// The text renderer keeps a link's destination so the plain-text part stays
// usable, which put URL slugs in front of the trigger-term list: a CTA
// pointing at /free-trial cost eight points for a word no reader ever sees.
func TestScoreIgnoresWordsInsideALinkDestination(t *testing.T) {
	plain := Score("Quick question", `<p>Hi Ana, worth a look?</p>`, "")
	slug := Score("Quick question", `<p>Hi Ana, <a href="https://example.com/free-trial">worth a look?</a></p>`, "")
	if slug.Score != plain.Score {
		t.Errorf("a URL slug changed the content score: %d vs %d (%v)", slug.Score, plain.Score, slug.Issues)
	}
}

// A stylesheet is markup machinery, never copy: a class named for a trigger
// term must not cost a designed email anything.
func TestScoreIgnoresAStylesheet(t *testing.T) {
	clean := Score("Quick question", "<p>Hi Ana, ten minutes on Thursday?</p>", "")
	styled := Score("Quick question", `<style>.free-trial-banner{color:red}</style><p>Hi Ana, ten minutes on Thursday?</p>`, "")
	if styled.Score != clean.Score {
		t.Errorf("a stylesheet changed the content score: %d vs %d (%v)", styled.Score, clean.Score, styled.Issues)
	}
}

func issueByCode(res ScoreResult, code string) (Issue, bool) {
	for _, i := range res.Issues {
		if i.Code == code {
			return i, true
		}
	}
	return Issue{}, false
}

// A score with no location is advice nobody can act on: the writer is told
// three trigger terms exist and left to hunt for them.
func TestScoreLocatesTriggerTermsInTheSubject(t *testing.T) {
	res := Score("Your FREE bonus inside", "<p>Hi Ada, worth a chat?</p>", "Hi Ada, worth a chat?")
	issue, ok := issueByCode(res, "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger terms not flagged: %+v", res.Issues)
	}
	if issue.Field != FieldSubject {
		t.Errorf("issue field = %q, want subject", issue.Field)
	}
	if len(issue.Spans) != 2 {
		t.Fatalf("got %d spans, want one per term: %+v", len(issue.Spans), issue.Spans)
	}
	for _, s := range issue.Spans {
		if s.Field != FieldSubject {
			t.Errorf("span %q sits in %q, want subject", s.Text, s.Field)
		}
		if s.Excerpt != "Your FREE bonus inside" {
			t.Errorf("span %q quoted %q, want the whole subject line", s.Text, s.Excerpt)
		}
	}
	if issue.Spans[0].Text != "FREE" {
		t.Errorf("first span = %q, want the term as the writer typed it", issue.Spans[0].Text)
	}
	if issue.Suggestion == "" {
		t.Error("no suggestion on a trigger-term issue")
	}
}

func TestScoreLocatesTriggerTermsInTheBodyByLine(t *testing.T) {
	body := "Hi Ada,\n\nThis is a limited time offer.\n\nWorth a chat?"
	res := Score("Quick question", "", body)
	issue, ok := issueByCode(res, "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger phrase not flagged: %+v", res.Issues)
	}
	if issue.Field != FieldBody {
		t.Errorf("issue field = %q, want body", issue.Field)
	}
	span := issue.Spans[0]
	if span.Line != 3 {
		t.Errorf("span line = %d, want the third line", span.Line)
	}
	if span.Excerpt != "This is a limited time offer." {
		t.Errorf("excerpt = %q, want the sentence it sits in", span.Excerpt)
	}
}

// A term in both halves counts once, as it always has, and is shown where the
// reader meets it first.
func TestScoreCountsATermInBothHalvesOnce(t *testing.T) {
	res := Score("A free look", "", "Here is a free look at it.")
	issue, ok := issueByCode(res, "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger term not flagged: %+v", res.Issues)
	}
	if len(issue.Spans) != 1 {
		t.Fatalf("got %d spans for one distinct term: %+v", len(issue.Spans), issue.Spans)
	}
	if issue.Spans[0].Field != FieldSubject {
		t.Errorf("span field = %q, want the subject occurrence", issue.Spans[0].Field)
	}
	if res.Score != 92 {
		t.Errorf("score = %d, want one term's deduction only", res.Score)
	}
}

// The trigger pass reads a URL-stripped copy of the text; quoting that copy
// would show the writer a line with their own links blanked out.
func TestScoreExcerptKeepsTheLinkTheTriggerPassStripped(t *testing.T) {
	body := "A free look: https://example.com/pricing"
	res := Score("Quick question", "", body)
	issue, ok := issueByCode(res, "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger term not flagged: %+v", res.Issues)
	}
	if issue.Spans[0].Excerpt != body {
		t.Errorf("excerpt = %q, want the line as written", issue.Spans[0].Excerpt)
	}
}

func TestScoreLocatesStackedPunctuationAndImages(t *testing.T) {
	res := Score("Hurry!!", "<p>Ready?!</p>", "Ready?!")
	issue, ok := issueByCode(res, "stacked_punctuation")
	if !ok {
		t.Fatalf("stacked punctuation not flagged: %+v", res.Issues)
	}
	if issue.Field != "" {
		t.Errorf("field = %q, want empty when the issue straddles both halves", issue.Field)
	}
	if len(issue.Spans) != 2 {
		t.Fatalf("got %d spans, want one per run: %+v", len(issue.Spans), issue.Spans)
	}
	if issue.Spans[0].Field != FieldSubject || issue.Spans[1].Field != FieldBody {
		t.Errorf("spans = %+v, want the subject one first", issue.Spans)
	}
}

func TestScoreLinkIssueNamesTheDestinations(t *testing.T) {
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	html := "<p>" + body + "</p><p>" +
		`<a href="https://a.com/1">one</a> <a href="https://b.com/2">two</a> ` +
		`<a href="https://c.com/3">three</a> <a href="https://d.com/4">four</a></p>`
	res := Score("Quick question", html, body)
	issue, ok := issueByCode(res, "too_many_links")
	if !ok {
		t.Fatalf("four links not flagged: %+v", res.Issues)
	}
	if len(issue.Spans) != 4 {
		t.Fatalf("got %d spans, want one per link: %+v", len(issue.Spans), issue.Spans)
	}
	if issue.Spans[0].Text != "https://a.com/1" {
		t.Errorf("first span = %q, want the destination", issue.Spans[0].Text)
	}
}

// Trimming the displayed spans must never move the score, so the count that
// drives the deduction is taken before the cap.
func TestScoreCapsSpansWithoutMovingTheScore(t *testing.T) {
	body := strings.Repeat("A real sentence about the recipient's work. ", 10)
	links := ""
	for i := 0; i < 14; i++ {
		links += fmt.Sprintf(`<a href="https://example.com/%d">link</a> `, i)
	}
	res := Score("Quick question", "<p>"+body+"</p><p>"+links+"</p>", body)
	issue, ok := issueByCode(res, "too_many_links")
	if !ok {
		t.Fatalf("fourteen links not flagged: %+v", res.Issues)
	}
	if len(issue.Spans) != maxSpans {
		t.Errorf("got %d spans, want them capped at %d", len(issue.Spans), maxSpans)
	}
	if !strings.HasPrefix(issue.Message, "14 links") {
		t.Errorf("message = %q, want it to count all fourteen", issue.Message)
	}
	// 14 links is 11 over the allowance, well past the 20-point cap.
	if res.Score != 80 {
		t.Errorf("score = %d, want the capped 20-point deduction", res.Score)
	}
}

func TestScoreFieldsTheIssuesWithNothingToQuote(t *testing.T) {
	res := Score("", "", "")
	for _, code := range []string{"empty_subject", "empty_body"} {
		issue, ok := issueByCode(res, code)
		if !ok {
			t.Fatalf("%s not flagged: %+v", code, res.Issues)
		}
		if issue.Field == "" {
			t.Errorf("%s has no field", code)
		}
		if issue.Suggestion == "" {
			t.Errorf("%s has no suggestion", code)
		}
	}
}

// The preflight dialog and the campaign feed both print one line about the
// worst thing in a step's copy, and that line has to say which box to open.
func TestLeadIssueNamesTheField(t *testing.T) {
	if got := LeadIssue(Score("FREE CASH PRIZE", "<p>Hi Ada, worth a chat?</p>", "Hi Ada, worth a chat?")); !strings.HasPrefix(got, "Subject: ") {
		t.Errorf("lead issue = %q, want it to name the subject", got)
	}
	if got := LeadIssue(Score("Quick question", "", "Here is a limited time offer.")); !strings.HasPrefix(got, "Body: ") {
		t.Errorf("lead issue = %q, want it to name the body", got)
	}
	if got := LeadIssue(Score("Quick question", "<p>Hi Ada, worth a chat?</p>", "Hi Ada, worth a chat?")); got != "" {
		t.Errorf("lead issue = %q on clean copy, want nothing", got)
	}
}

// A high-severity issue outranks a warning even when the warning came first.
func TestLeadIssuePrefersTheHighSeverityOne(t *testing.T) {
	res := ScoreResult{Issues: []Issue{
		{Severity: "warn", Code: "stacked_punctuation", Message: "Stacked punctuation."},
		{Severity: "high", Code: "empty_body", Field: FieldBody, Message: "Body has no text content."},
	}}
	if got := LeadIssue(res); got != "Body: Body has no text content." {
		t.Errorf("lead issue = %q, want the high-severity one", got)
	}
}

// A rune can change byte length when it is lowercased (U+0130 is two bytes and
// folds to one, U+212A is three), so an offset found in the folded copy walks
// off the original past the first one. Slicing on it quoted bytes the writer
// never typed, and could cut a rune in half into invalid UTF-8.
func TestScoreQuotesTheRightBytesAcrossACaseFold(t *testing.T) {
	for _, tc := range []struct {
		subject string
		want    string
	}{
		{"İİİ free offer", "free"},
		{"KKK cash prize", "cash"},
		{"café free trial", "free"},
		{"🎉 free bonus", "free"},
	} {
		issue, ok := issueByCode(Score(tc.subject, "", "Hi Ada, worth a chat?"), "spam_trigger_terms")
		if !ok {
			t.Fatalf("%q: trigger term not flagged", tc.subject)
		}
		span := issue.Spans[0]
		if !utf8.ValidString(span.Text) {
			t.Errorf("%q: span text is not valid UTF-8: %q", tc.subject, span.Text)
		}
		if span.Text != tc.want {
			t.Errorf("%q: quoted %q, want %q", tc.subject, span.Text, tc.want)
		}
		if span.Excerpt != tc.subject {
			t.Errorf("%q: excerpt = %q", tc.subject, span.Excerpt)
		}
	}
}

// The same offsets carry the line number, so a body with a fold-shifting rune
// above the trigger term must still point at the right line.
func TestScoreLinesSurviveACaseFold(t *testing.T) {
	body := "Hi İIrem,\n\nStraße KKK notes.\n\nHere is a free look."
	issue, ok := issueByCode(Score("Quick question", "", body), "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger term not flagged")
	}
	span := issue.Spans[0]
	if span.Text != "free" {
		t.Errorf("quoted %q, want %q", span.Text, "free")
	}
	if span.Line != 5 {
		t.Errorf("line = %d, want 5", span.Line)
	}
	if span.Excerpt != "Here is a free look." {
		t.Errorf("excerpt = %q", span.Excerpt)
	}
}

// The field is read before the span list is trimmed. A subject-first list of
// more than maxSpans in one half would otherwise lose every body span to the
// cap and label a both-halves issue as a subject-only one.
func TestScoreFieldSurvivesTheSpanCap(t *testing.T) {
	subject := "free cash prize bonus discount promo sale deal loan credit"
	body := "This is a limited time offer."
	issue, ok := issueByCode(Score(subject, "", body), "spam_trigger_terms")
	if !ok {
		t.Fatalf("trigger terms not flagged")
	}
	if len(issue.Spans) != maxSpans {
		t.Fatalf("got %d spans, want them capped at %d", len(issue.Spans), maxSpans)
	}
	if issue.Field != "" {
		t.Errorf("field = %q, want empty: the terms straddle both halves", issue.Field)
	}
}

// The message names the terms and the field names the place, so the one-line
// summary does not read "Body: 3 spam-trigger term(s) found in subject/body".
func TestLeadIssueDoesNotRepeatTheLocation(t *testing.T) {
	got := LeadIssue(Score("Quick question", "", "Here is a free look at it."))
	if got != "Body: 1 spam-trigger term(s): free." {
		t.Errorf("lead issue = %q", got)
	}
}

// Terms in both halves have no single field, so the summary says both rather
// than dropping the location entirely.
func TestLeadIssueNamesBothHalvesWhenTheIssueStraddles(t *testing.T) {
	got := LeadIssue(Score("A free look", "", "This is a limited time offer."))
	if !strings.HasPrefix(got, "Subject and body: ") {
		t.Errorf("lead issue = %q, want it to name both halves", got)
	}
}
