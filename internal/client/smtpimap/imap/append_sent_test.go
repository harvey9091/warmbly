package imap

import (
	"strings"
	"testing"
)

func TestLeaf(t *testing.T) {
	cases := map[string]string{
		"Sent":            "Sent",
		"INBOX.Sent":      "Sent",
		"INBOX/Sent Mail": "Sent Mail",
		"":                "",
		"Sent/":           "Sent/",
	}
	for in, want := range cases {
		if got := leaf(in); got != want {
			t.Fatalf("leaf(%q) = %q want %q", in, got, want)
		}
	}
}

func TestImapSentCoversCommonNames(t *testing.T) {
	// The name list is the fallback when a server does not advertise the
	// RFC 6154 \Sent attribute; these are what the common servers call it.
	// Localized names matter as much as the English ones: a server that
	// advertises no \Sent attribute reports the folder in its owner's
	// language, and an unmatched Sent folder means no sent copies at all.
	for _, name := range []string{"Sent", "Sent Items", "Sent Mail", "Gesendete Elemente", "Éléments envoyés", "Enviados", "Elküldött elemek"} {
		if !matchesFolderName(strings.ToLower(name), ImapSent) {
			t.Fatalf("ImapSent does not cover %q", name)
		}
	}
}
