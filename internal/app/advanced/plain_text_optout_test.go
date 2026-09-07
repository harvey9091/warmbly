package advanced

import (
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// The workspace default: a reply line, no link anywhere.
func replyLineSettings() models.UnsubscribeSettings {
	return models.DefaultAdvancedOutreachSettings().Unsubscribe
}

func TestPlainTextOptOutPassesWithTheReplyLine(t *testing.T) {
	seqs := []models.Sequence{emailStep(0, "Quick question", "Hi there, worth a chat?")}
	if got := plainTextOptOutResult(stepPlacingUnsubscribeLink(seqs)); !got.Passed {
		t.Fatalf("reply-line opt-out should pass on a plain-text campaign: %+v", got)
	}
}

func TestPlainTextOptOutWarnsOnLinkMode(t *testing.T) {
	c := &models.Campaign{TextOnly: true, UnsubscribeMode: "link"}
	if replyLineSettings().Effective(c.UnsubscribeMode).Mode != models.UnsubscribeModeLink {
		t.Fatal("campaign link mode should win over the workspace reply line")
	}
	got := plainTextOptOutResult("the opt-out line is set to Unsubscribe link")
	if got.Passed || got.Severity != "warning" {
		t.Fatalf("link mode on a plain-text campaign should warn: %+v", got)
	}
	if !strings.Contains(got.Message, "opt-out line is set to Unsubscribe link") || got.Remediation == "" {
		t.Fatalf("warning should name the cause and a way out: %+v", got)
	}
}

func TestPlainTextOptOutCampaignModeOverridesTheWorkspaceLink(t *testing.T) {
	// Workspace is on link mode, this campaign opted back to the reply line.
	workspace := models.UnsubscribeSettings{Mode: models.UnsubscribeModeLink}
	c := &models.Campaign{TextOnly: true, UnsubscribeMode: "text"}
	if workspace.Effective(c.UnsubscribeMode).Mode == models.UnsubscribeModeLink {
		t.Fatal("campaign override to the reply line should not resolve to link mode")
	}
}

func TestStepPlacingUnsubscribeLinkNamesTheStep(t *testing.T) {
	seqs := []models.Sequence{
		emailStep(0, "Quick question", "Hi there"),
		{Kind: "wait", Position: 1},
		emailStep(2, "Following up", "Not for you? "+models.UnsubscribeLinkToken),
	}
	// Unnamed steps fall back to their place in builder order, not `position`,
	// which is 0-based on some campaigns and 1-based on others.
	if got := stepPlacingUnsubscribeLink(seqs); !strings.Contains(got, "step 3 places the unsubscribe link variable") {
		t.Fatalf("warning should name the step: %q", got)
	}

	named := []models.Sequence{{Kind: "email", Name: "Follow-up", BodyPlain: models.UnsubscribeLinkToken}}
	if got := stepPlacingUnsubscribeLink(named); !strings.Contains(got, `the step "Follow-up" places`) {
		t.Fatalf("a named step should be named: %q", got)
	}
}

func TestStepPlacingUnsubscribeLinkIgnoresNonEmailSteps(t *testing.T) {
	seqs := []models.Sequence{
		{Kind: "action", Position: 0, BodyPlain: models.UnsubscribeLinkToken},
		emailStep(1, "Quick question", "Reply and I'll stop emailing."),
	}
	if got := stepPlacingUnsubscribeLink(seqs); got != "" {
		t.Fatalf("a non-email node's config must not be read as copy: %q", got)
	}
}

// A plain-text send ships body_plain, or the text of body_html when there is
// none. A token that only ever sits in an HTML attribute never reaches the
// recipient, so warning about it would be a false alarm.
func TestStepPlacingUnsubscribeLinkIgnoresAnAttributeOnlyToken(t *testing.T) {
	attrOnly := []models.Sequence{{
		Kind:     "email",
		Name:     "Intro",
		Subject:  "Quick question",
		BodyHTML: `<p>Or <a href="` + models.UnsubscribeLinkToken + `">no thanks</a>.</p>`,
	}}
	if got := stepPlacingUnsubscribeLink(attrOnly); got != "" {
		t.Fatalf("a token only in an href never reaches a plain-text recipient: %q", got)
	}

	// The composer's own chip serializes as text inside a span, which the plain
	// part does carry.
	chip := []models.Sequence{{
		Kind:     "email",
		Name:     "Intro",
		BodyHTML: `<p>Or <span data-var>` + models.UnsubscribeLinkToken + `</span>.</p>`,
	}}
	if got := stepPlacingUnsubscribeLink(chip); got == "" {
		t.Fatal("a chip token does reach the plain part and should warn")
	}

	// An explicit plain body wins over the HTML, exactly as the send path does.
	explicit := []models.Sequence{{
		Kind:      "email",
		Name:      "Intro",
		BodyHTML:  `<p>Or <span data-var>` + models.UnsubscribeLinkToken + `</span>.</p>`,
		BodyPlain: "Or reply and I'll stop.",
	}}
	if got := stepPlacingUnsubscribeLink(explicit); got != "" {
		t.Fatalf("the plain body is what ships; the HTML must not be scanned: %q", got)
	}
}

func TestStepPlacingUnsubscribeLinkCatchesTheSubject(t *testing.T) {
	seqs := []models.Sequence{{Kind: "email", Name: "Intro", Subject: "Out? " + models.UnsubscribeLinkToken}}
	if got := stepPlacingUnsubscribeLink(seqs); got == "" {
		t.Fatal("a subject carries no HTML at all, so its token is always raw")
	}
}
