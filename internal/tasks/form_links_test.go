package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

type stubMinter struct {
	known map[string]string // public_id -> personal URL
	calls int
}

func (m *stubMinter) MintForContact(_ context.Context, _ uuid.UUID, publicID string, _ uuid.UUID, _ *uuid.UUID) (string, string, bool) {
	m.calls++
	url, ok := m.known[publicID]
	return url, "https://forms.example.com/f/" + publicID, ok
}

func linkFixture(minter FormLinkMinter) (*tasksService, *models.Campaign, *models.Contact) {
	s := &tasksService{formLinks: minter}
	return s, &models.Campaign{ID: uuid.New()}, &models.Contact{ID: uuid.New(), FirstName: "Jane"}
}

func TestResolveFormLinksSubstitutesPersonalURL(t *testing.T) {
	minter := &stubMinter{known: map[string]string{"abc123": "https://forms.example.com/f/abc123?t=tok"}}
	s, campaign, contact := linkFixture(minter)

	body := `Hi {{.FirstName}}, book here: {{form_link:abc123}} or later {{ form_link:abc123 }}.`
	s.resolveFormLinks(context.Background(), uuid.New(), campaign, contact, &body)

	want := `Hi {{.FirstName}}, book here: https://forms.example.com/f/abc123?t=tok or later https://forms.example.com/f/abc123?t=tok.`
	if body != want {
		t.Fatalf("body = %q", body)
	}
	// Template variables must survive untouched for RenderTemplate.
	if rendered := RenderTemplate(body, models.Contact{FirstName: "Jane"}); rendered == body {
		t.Fatal("template variable was mangled by link resolution")
	}
}

func TestResolveFormLinksUnknownFormDropsMarker(t *testing.T) {
	s, campaign, contact := linkFixture(&stubMinter{known: map[string]string{}})
	body := "Try {{form_link:nosuchform}} now"
	s.resolveFormLinks(context.Background(), uuid.New(), campaign, contact, &body)
	if body != "Try  now" {
		t.Fatalf("unresolved marker not dropped: %q", body)
	}
}

func TestResolveFormLinksNilMinterAndMalformedMarkers(t *testing.T) {
	s, campaign, contact := linkFixture(nil)
	body := "A {{form_link:abc123}} B"
	s.resolveFormLinks(context.Background(), uuid.New(), campaign, contact, &body)
	if body != "A  B" {
		t.Fatalf("nil minter should drop the marker, got %q", body)
	}

	minter := &stubMinter{known: map[string]string{"abc123": "u"}}
	s2, campaign2, contact2 := linkFixture(minter)
	// Uppercase ids, missing id and plain variables never match the marker.
	untouched := "{{form_link:ABC}} {{form_link:}} {{.FirstName}} {form_link:abc123}"
	got := untouched
	s2.resolveFormLinks(context.Background(), uuid.New(), campaign2, contact2, &got)
	if got != untouched || minter.calls != 0 {
		t.Fatalf("malformed markers were touched: %q (calls %d)", got, minter.calls)
	}
}
