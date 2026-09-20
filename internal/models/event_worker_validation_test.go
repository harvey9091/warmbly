package models

import "testing"

func TestGoogleMailHost(t *testing.T) {
	for host, want := range map[string]bool{
		"smtp.gmail.com":         true,
		"IMAP.Gmail.Com":         true,
		"smtp.googlemail.com":    true,
		"smtp-relay.gmail.com":   true,
		"aspmx.l.google.com":     true,
		" smtp.gmail.com. ":      true,
		"gmail.com":              true,
		"smtp.gmail.com.evil.io": false,
		"notgmail.com":           false,
		"smtp.office365.com":     false,
		"":                       false,
	} {
		if got := GoogleMailHost(host); got != want {
			t.Errorf("GoogleMailHost(%q) = %v, want %v", host, got, want)
		}
	}
}
