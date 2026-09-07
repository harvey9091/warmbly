package replyclassify

import "testing"

func TestIsOptOut(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		body    string
		want    bool
	}{
		{"plain", "", "Please unsubscribe me from this list.", true},
		{"remove me", "Re: hi", "remove me from your list", true},
		{"stop emailing", "", "Stop emailing me.", true},
		{"hyphen", "", "I'd like to opt-out.", true},
		{"word boundary", "", "Stop by our booth next week!", false},
		{"substring", "", "We have an unsubscribed model in beta", false},
		{"quoted footer only", "", "Sounds interesting, let's talk.\n\nOn Tue, Sep 1, 2026 Jane <jane@x.com> wrote:\n> If this isn't relevant, just reply and I'll stop.\n> Unsubscribe: https://example.com/u/abc", false},
		{"quote lines only", "", "Yes please\n> unsubscribe here", false},
		{"before quote", "", "Please remove me\n\nOn Mon Jane wrote:\n> hello", true},
		{"outlook header", "", "not interested\r\nFrom: Jane\r\nSent: Monday\r\nunsubscribe", false},
		{"curly apostrophe", "", "Please don\u2019t email me again.", true},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		if got := IsOptOut(c.subject, c.body); got != c.want {
			t.Errorf("%s: IsOptOut=%v want %v", c.name, got, c.want)
		}
	}
}

func TestLexiconIgnoresQuotedHistory(t *testing.T) {
	r, ok := classifyLexicon(Input{BodyText: "Sounds good, let's talk.\n\nOn Mon, X wrote:\n> reply STOP or unsubscribe to opt out"})
	if !ok || r.Class != ClassPositive {
		t.Fatalf("got %+v ok=%v, want positive", r, ok)
	}
}
