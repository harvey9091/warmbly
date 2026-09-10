package spamcheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/pkg/generation"
	"github.com/warmbly/warmbly/internal/pkg/warmlint"
)

const testBody = "Hi Ada,\n\nThis is a limited time offer you cannot miss.\n\nWorth ten minutes?"

// analyzed runs the parse + verify halves the way Analyze does, over a model
// response, so the tests exercise exactly what a real completion goes through.
func analyzed(t *testing.T, response, subject, body string) *Result {
	t.Helper()
	res := parse(response)
	verify(res, subject, body)
	return res
}

func TestVerifyAnchorsAQuoteInTheBody(t *testing.T) {
	res := analyzed(t, `{
	  "score": 55,
	  "verdict": "Reads like a promotion.",
	  "findings": [
	    {"severity":"high","field":"body","text":"limited time offer","issue":"Urgency wording filters weight heavily.","suggestion":"Say what changed instead."}
	  ]
	}`, "Quick question", testBody)

	if len(res.Findings) != 1 {
		t.Fatalf("got %d findings, want the quoted one kept: %+v", len(res.Findings), res.Findings)
	}
	f := res.Findings[0]
	if f.Line != 3 {
		t.Errorf("line = %d, want the third line of the body", f.Line)
	}
	if f.Excerpt != "This is a limited time offer you cannot miss." {
		t.Errorf("excerpt = %q, want the sentence around the quote", f.Excerpt)
	}
	if res.Score != 55 {
		t.Errorf("score = %d, want the model's own", res.Score)
	}
}

// A panel that points at a sentence the writer never wrote is worse than one
// that points at nothing, so an unverifiable quote loses the quote and keeps
// the advice.
func TestVerifyDropsAQuoteThatIsNotInTheCopy(t *testing.T) {
	res := analyzed(t, `{
	  "score": 60,
	  "findings": [
	    {"severity":"warn","field":"body","text":"ACT NOW while stocks last","issue":"Hype wording hurts.","suggestion":"Cut it."}
	  ]
	}`, "Quick question", testBody)

	if len(res.Findings) != 1 {
		t.Fatalf("got %d findings, want the advice kept: %+v", len(res.Findings), res.Findings)
	}
	f := res.Findings[0]
	if f.Text != "" || f.Line != 0 || f.Excerpt != "" {
		t.Errorf("invented quote survived verification: %+v", f)
	}
	if f.Issue == "" {
		t.Error("the advice was dropped along with the quote")
	}
}

// The analysis can be right while the label is wrong. Sending the writer to the
// wrong box for a real problem is a worse outcome than correcting the label.
func TestVerifyCorrectsTheFieldWhenTheQuoteIsInTheOtherHalf(t *testing.T) {
	res := analyzed(t, `{
	  "score": 70,
	  "findings": [
	    {"severity":"warn","field":"subject","text":"limited time offer","issue":"Urgency wording.","suggestion":"Cut it."}
	  ]
	}`, "Quick question", testBody)

	if res.Findings[0].Field != "body" {
		t.Errorf("field = %q, want it corrected to body", res.Findings[0].Field)
	}
}

// A model retyping a quote tends to normalize its capitalization.
func TestVerifyMatchesAQuoteCaseInsensitively(t *testing.T) {
	res := analyzed(t, `{
	  "score": 70,
	  "findings": [
	    {"severity":"warn","field":"subject","text":"free bonus","issue":"Trigger wording.","suggestion":"Cut it."}
	  ]
	}`, "Your FREE BONUS inside", testBody)

	f := res.Findings[0]
	if f.Field != "subject" || f.Excerpt != "Your FREE BONUS inside" {
		t.Errorf("quote was not matched against the subject: %+v", f)
	}
	// The fragment is documented as quoted character for character and the
	// editor highlights it, so it has to come back in the copy's own casing.
	if f.Text != "FREE BONUS" {
		t.Errorf("quoted %q, want the copy's own %q", f.Text, "FREE BONUS")
	}
}

// The quote is sliced out of the copy, so the offsets have to survive a rune
// that changes byte length when it is lowercased.
func TestVerifyQuotesTheCopyAcrossACaseFold(t *testing.T) {
	res := analyzed(t, `{
	  "score": 60,
	  "findings": [
	    {"severity":"warn","field":"subject","text":"free bonus","issue":"Trigger wording."}
	  ]
	}`, "İstanbul FREE BONUS inside", testBody)

	if got := res.Findings[0].Text; got != "FREE BONUS" {
		t.Errorf("quoted %q, want %q", got, "FREE BONUS")
	}
}

func TestVerifyOrdersFindingsBySeverity(t *testing.T) {
	res := analyzed(t, `{
	  "score": 40,
	  "findings": [
	    {"severity":"low","field":"body","issue":"Third."},
	    {"severity":"medium","field":"body","issue":"Second."},
	    {"severity":"critical","field":"body","issue":"First."}
	  ]
	}`, "Quick question", testBody)

	got := []string{}
	for _, f := range res.Findings {
		got = append(got, f.Issue+"/"+f.Severity)
	}
	want := "First./high Second./warn Third./info"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestParseToleratesAFencedResponse(t *testing.T) {
	res := parse("Here you go:\n```json\n{\"score\": 88, \"verdict\": \"Fine.\"}\n```\n")
	if res.Score != 88 || res.Verdict != "Fine." {
		t.Errorf("fenced JSON was not read: %+v", res)
	}
}

// A response nothing can be read out of must not render as 0/100: the panel
// still has the rules pass, and a fabricated zero would contradict it.
func TestParseRefusesToInventAScore(t *testing.T) {
	res := parse("I could not analyze that.")
	if res.Score != -1 {
		t.Errorf("score = %d, want the missing-score marker", res.Score)
	}
	if res.Findings == nil {
		t.Error("findings must be an empty list, never null in JSON")
	}
}

// A model that returned findings but no usable score still has to produce one,
// or Re-check has nothing to compare.
func TestVerifyDerivesAScoreWhenTheModelOmittedIt(t *testing.T) {
	res := analyzed(t, `{
	  "findings": [
	    {"severity":"high","field":"body","issue":"One."},
	    {"severity":"warn","field":"body","issue":"Two."}
	  ]
	}`, "Quick question", testBody)

	if res.Score != 70 {
		t.Errorf("score = %d, want one derived from the findings", res.Score)
	}
}

func TestVerifyDropsDuplicateFindings(t *testing.T) {
	res := analyzed(t, `{
	  "score": 50,
	  "findings": [
	    {"severity":"warn","field":"body","text":"limited time offer","issue":"Urgency wording."},
	    {"severity":"warn","field":"body","text":"limited time offer","issue":"Urgency wording."}
	  ]
	}`, "Quick question", testBody)

	if len(res.Findings) != 1 {
		t.Errorf("got %d findings, want the repeat dropped: %+v", len(res.Findings), res.Findings)
	}
}

func TestParseBoundsTheResponse(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"score": 20, "improvements": [`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"tip"`)
	}
	b.WriteString(`], "findings": [`)
	for i := 0; i < 30; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"severity":"warn","field":"body","issue":"something"}`)
	}
	b.WriteString(`]}`)

	res := parse(b.String())
	if len(res.Improvements) != maxImprovements {
		t.Errorf("kept %d improvements, want %d", len(res.Improvements), maxImprovements)
	}
	if len(res.Findings) != maxFindings {
		t.Errorf("kept %d findings, want %d", len(res.Findings), maxFindings)
	}
}

// stubProvider answers with a canned completion, so the tests can drive Analyze
// end to end without a provider.
type stubProvider struct {
	reply string
	got   generation.CompletionRequest
}

func (s *stubProvider) RunAgent(context.Context, generation.AgentRequest) (*generation.AgentResult, error) {
	return nil, errors.New("not used")
}

func (s *stubProvider) Complete(_ context.Context, req generation.CompletionRequest) (*generation.WritingResult, error) {
	s.got = req
	return &generation.WritingResult{Text: s.reply, Model: "stub-model", TokensUsed: 321}, nil
}

func (s *stubProvider) ModelForTier(bool) string { return "stub-model" }
func (s *stubProvider) Name() string             { return "stub" }
func (s *stubProvider) IsLocal() bool            { return true }

// A response nothing could be read out of must fail the call, which refunds the
// credit, rather than render as a confident 100/100 on copy nothing read.
func TestAnalyzeRefusesAnUnreadableResponse(t *testing.T) {
	_, err := Analyze(context.Background(), &stubProvider{reply: "I could not analyze that."}, "stub-model", Input{
		Subject:   "Quick question",
		BodyPlain: testBody,
	})
	if err == nil {
		t.Fatal("an unreadable response was accepted as an analysis")
	}
}

func TestAnalyzeIsDeterministicAndGroundsOnTheRulesPass(t *testing.T) {
	p := &stubProvider{reply: `{"score": 62, "verdict": "Reads promotional."}`}
	rules := warmlint.Score("Your FREE bonus", "", testBody)

	res, err := Analyze(context.Background(), p, "stub-model", Input{
		Subject:   "Your FREE bonus",
		BodyPlain: testBody,
		Rules:     rules,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Score != 62 || res.Model != "stub-model" || res.TokensUsed != 321 {
		t.Errorf("result did not carry the completion through: %+v", res)
	}
	// Re-check compares this score with the last one, so a sampled score would
	// move on copy the writer never touched.
	if p.got.Temperature == nil || *p.got.Temperature != 0 {
		t.Errorf("temperature = %v, want pinned to 0", p.got.Temperature)
	}
	// The model is told what the rules pass already proved, so the two halves of
	// the panel cannot contradict each other.
	if !strings.Contains(p.got.Prompt, "spam_trigger_terms") || !strings.Contains(p.got.Prompt, `"FREE"`) {
		t.Errorf("prompt did not carry the rule-based findings:\n%s", p.got.Prompt)
	}
}

func TestAnalyzeTruncatesAVeryLongBody(t *testing.T) {
	p := &stubProvider{reply: `{"score": 80, "verdict": "Fine."}`}
	if _, err := Analyze(context.Background(), p, "stub-model", Input{
		Subject:   "Quick question",
		BodyPlain: strings.Repeat("word ", 40000),
	}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len([]rune(p.got.Prompt)) > maxBodyRunes+2000 {
		t.Errorf("prompt is %d runes, want the body bounded at %d", len([]rune(p.got.Prompt)), maxBodyRunes)
	}
}

// A quote longer than the cap is shortened to fit, and the shortened form still
// has to verify: an ellipsis on the end would never be found in the copy, so
// every long quote would silently lose the fragment it named.
func TestVerifyKeepsALongQuoteThatHadToBeShortened(t *testing.T) {
	sentence := strings.Repeat("this is a long promotional sentence that keeps going ", 12)
	body := "Hi Ada,\n\n" + sentence + "\n\nWorth ten minutes?"
	quoted, err := json.Marshal(sentence)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res := analyzed(t, `{"score": 40, "findings": [
	    {"severity":"high","field":"body","text":`+string(quoted)+`,"issue":"Too long and too promotional."}
	]}`, "Quick question", body)

	f := res.Findings[0]
	if f.Text == "" {
		t.Fatal("a quote that only needed shortening was dropped as unverifiable")
	}
	if len([]rune(f.Text)) > maxTextRunes {
		t.Errorf("quote is %d runes, want it bounded at %d", len([]rune(f.Text)), maxTextRunes)
	}
	if f.Line != 3 {
		t.Errorf("line = %d, want the line the sentence sits on", f.Line)
	}
}

// A reply carrying only prose is not an analysis. Deriving a score from its
// empty finding list put a confident 100/100 above a verdict saying the
// opposite, and charged for it.
func TestAnalyzeRefusesAVerdictWithNothingBehindIt(t *testing.T) {
	_, err := Analyze(context.Background(), &stubProvider{
		reply: `{"verdict": "This reads like a bulk promotion and will land in spam."}`,
	}, "stub-model", Input{Subject: "Quick question", BodyPlain: testBody})
	if err == nil {
		t.Fatal("a verdict with no score and no findings was accepted as an analysis")
	}
}

// A score of 0 is a real answer, not a missing one.
func TestAnalyzeAcceptsAZeroScore(t *testing.T) {
	res, err := Analyze(context.Background(), &stubProvider{
		reply: `{"score": 0, "verdict": "Every signal here is bad."}`,
	}, "stub-model", Input{Subject: "FREE CASH", BodyPlain: testBody})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Score != 0 {
		t.Errorf("score = %d, want the model's own 0", res.Score)
	}
}

// A finding the model left unlabelled must not be given a half at random. With
// a quote, verify anchors it; without one, it carries no field rather than a
// badge sending the writer to the wrong box.
func TestVerifyDoesNotGuessAnUnlabelledField(t *testing.T) {
	res := analyzed(t, `{
	  "score": 60,
	  "findings": [
	    {"severity":"warn","text":"limited time offer","issue":"Anchored, so the half is known."},
	    {"severity":"warn","issue":"Nothing to anchor and no label."}
	  ]
	}`, "Quick question", testBody)

	if len(res.Findings) != 2 {
		t.Fatalf("got %d findings: %+v", len(res.Findings), res.Findings)
	}
	if res.Findings[0].Field != "body" {
		t.Errorf("anchored finding field = %q, want body", res.Findings[0].Field)
	}
	if res.Findings[1].Field != "" {
		t.Errorf("unanchored, unlabelled finding field = %q, want empty", res.Findings[1].Field)
	}
}

// category is documented as a closed set, so a response cannot be allowed to
// break the schema it is validated against.
func TestParseKeepsCategoryInsideItsEnum(t *testing.T) {
	res := parse(`{"score": 50, "findings": [
	    {"severity":"warn","field":"body","issue":"a","category":"Trigger Word"},
	    {"severity":"warn","field":"body","issue":"b","category":"spammy vibes"},
	    {"severity":"warn","field":"body","issue":"c","category":"links"}
	]}`)
	got := []string{}
	for _, f := range res.Findings {
		got = append(got, f.Category)
	}
	if strings.Join(got, ",") != "trigger_word,,links" {
		t.Errorf("categories = %q, want the unknown one dropped", strings.Join(got, ","))
	}
}
