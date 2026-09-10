// Package casefold makes a case-insensitive search able to quote what it found.
//
// The obvious way to search text without regard to case is to lowercase both
// sides and take strings.Index. The offset that comes back addresses the folded
// copy, not the original, and the two do not line up: strings.ToLower maps rune
// by rune, and a rune can change byte length doing it (U+0130 is two bytes and
// folds to one, U+212A is three). Past the first such rune, slicing the original
// at a folded offset lands mid-rune. It quotes bytes the writer never typed, or
// cuts one in half into invalid UTF-8.
//
// Index hands back the map that fixes it, so a match found in the fold can be
// quoted from the source exactly as it was written.
package casefold

import (
	"strings"
	"unicode"
)

// Index folds s to lower case and returns the folded text together with, for
// every byte offset in it, the offset it came from in s. The map has one entry
// past the end, so the end of a match at the end of the text maps too.
//
// A nil map means the two are byte-for-byte aligned and offsets need no
// translation, which is the case for ASCII text and so for almost every call.
func Index(s string) (string, []int) {
	if isASCII(s) {
		return strings.ToLower(s), nil
	}
	var b strings.Builder
	b.Grow(len(s))
	offsets := make([]int, 0, len(s)+1)
	for i, r := range s {
		before := b.Len()
		b.WriteRune(unicode.ToLower(r))
		for n := b.Len() - before; n > 0; n-- {
			offsets = append(offsets, i)
		}
	}
	offsets = append(offsets, len(s))
	return b.String(), offsets
}

// Origin translates a byte offset in the folded copy back to the original. It
// returns -1 for an offset the map does not cover, which a caller must treat as
// "cannot quote this" rather than as position zero.
func Origin(offsets []int, at int) int {
	if offsets == nil {
		if at < 0 {
			return -1
		}
		return at
	}
	if at < 0 || at >= len(offsets) {
		return -1
	}
	return offsets[at]
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
