package repository

import "testing"

func TestNormalizeMailboxAddress(t *testing.T) {
	tests := map[string]string{
		"Jane Doe <Jane@Example.com>": "jane@example.com",
		"Jane Doe (Jane@Example.com)": "jane@example.com",
		" Jane@Example.com ":          "jane@example.com",
		"":                            "",
	}
	for input, want := range tests {
		if got := normalizeMailboxAddress(input); got != want {
			t.Errorf("normalizeMailboxAddress(%q) = %q, want %q", input, got, want)
		}
	}
}
