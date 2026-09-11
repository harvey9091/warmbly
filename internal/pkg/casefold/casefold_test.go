package casefold

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Everything here rests on Index folding exactly the way strings.ToLower does:
// callers search one string and count against the other. Invalid
// UTF-8 is in the table because a range loop and strings.Map both turn a bad
// byte into U+FFFD, which is three bytes where the original was one.
func TestIndexMatchesStringsToLower(t *testing.T) {
	cases := []string{
		"", "plain ascii text", "FREE CASH", "İİİ free", "KKK cash", "café",
		"🎉 free", "Straße", "ǅungla", "ﬁ ligature", "\xff\xfe bad bytes",
		"mixed İ \xc3\x28 free", strings.Repeat("İ", 40) + " free",
	}
	for _, s := range cases {
		lower, offsets := Index(s)
		if want := strings.ToLower(s); lower != want {
			t.Errorf("Index(%q) folded to %q, want %q", s, lower, want)
		}
		if offsets == nil {
			continue
		}
		if len(offsets) != len(lower)+1 {
			t.Errorf("Index(%q): %d offsets for %d bytes", s, len(offsets), len(lower))
			continue
		}
		// Every offset must land on a rune boundary in the original, or a
		// slice taken at it cuts a rune in half.
		for i, at := range offsets {
			if at < 0 || at > len(s) {
				t.Fatalf("Index(%q): offset %d out of range at %d", s, at, i)
			}
			if at < len(s) && !utf8.RuneStart(s[at]) {
				t.Errorf("Index(%q): offset %d at index %d is mid-rune", s, at, i)
			}
		}
		if offsets[len(offsets)-1] != len(s) {
			t.Errorf("Index(%q): last offset %d, want %d", s, offsets[len(offsets)-1], len(s))
		}
	}
}

// Origin must say "cannot quote this" rather than pointing at position zero,
// which would silently quote the start of the text for a match that moved out
// of range.
func TestOriginRefusesAnOffsetOutsideTheMap(t *testing.T) {
	_, offsets := Index("İ free")
	if got := Origin(offsets, len(offsets)); got != -1 {
		t.Errorf("Origin past the end = %d, want -1", got)
	}
	if got := Origin(offsets, -1); got != -1 {
		t.Errorf("Origin of a negative offset = %d, want -1", got)
	}
	if got := Origin(nil, -1); got != -1 {
		t.Errorf("Origin of a negative offset with no map = %d, want -1", got)
	}
}

// ASCII text needs no map, and the offsets pass straight through.
func TestIndexSkipsTheMapForASCII(t *testing.T) {
	lower, offsets := Index("FREE Cash")
	if lower != "free cash" {
		t.Errorf("folded to %q", lower)
	}
	if offsets != nil {
		t.Errorf("built a map for ASCII text: %v", offsets)
	}
	if got := Origin(offsets, 5); got != 5 {
		t.Errorf("Origin = %d, want the offset unchanged", got)
	}
}
