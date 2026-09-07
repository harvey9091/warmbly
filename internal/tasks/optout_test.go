package tasks

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

func TestUnsubscribeLinkIsNeverTracked(t *testing.T) {
	body := `<p>Hi <a href="https://acme.com/pricing">pricing</a> and <a href="https://api.example.com/unsubscribe/abc123">unsubscribe</a></p>`
	out, links := WrapLinksForTracking(body, uuid.New(), uuid.New(), "t.example.com")
	if len(links) != 1 || links[0].Destination != "https://acme.com/pricing" {
		t.Fatalf("expected only the pricing link to be ticketed, got %+v", links)
	}
	if !strings.Contains(out, `href="https://api.example.com/unsubscribe/abc123"`) {
		t.Fatalf("unsubscribe link was rewritten: %s", out)
	}
}

func TestOptOutFooter(t *testing.T) {
	text := models.UnsubscribeSettings{Mode: models.UnsubscribeModeText, Text: "Reply and I'll stop."}
	h, p := appendOptOut("<p>Hi</p>", "Hi", text, "")
	if !strings.Contains(h, "Reply and I&#39;ll stop.") || !strings.HasSuffix(p, "Reply and I'll stop.") {
		t.Fatalf("text footer missing: %q / %q", h, p)
	}

	link := models.UnsubscribeSettings{Mode: models.UnsubscribeModeLink, Text: "fallback", LinkIntro: "Not interested?", LinkText: "Unsubscribe"}
	h, p = appendOptOut("<p>Hi</p>", "Hi", link, "https://api.example.com/unsubscribe/tok")
	if !strings.Contains(h, `href="https://api.example.com/unsubscribe/tok"`) || !strings.Contains(p, "Unsubscribe: https://api.example.com/unsubscribe/tok") {
		t.Fatalf("link footer missing: %q / %q", h, p)
	}
	// No link to mint: link mode degrades to the text line, never to nothing.
	h, _ = appendOptOut("<p>Hi</p>", "Hi", link, "")
	if !strings.Contains(h, "fallback") {
		t.Fatalf("link mode without a link should fall back to text: %q", h)
	}
	h, p = appendOptOut("<p>Hi</p>", "Hi", models.UnsubscribeSettings{Mode: models.UnsubscribeModeOff}, "x")
	if h != "<p>Hi</p>" || p != "Hi" {
		t.Fatalf("off mode changed the body: %q / %q", h, p)
	}
}
