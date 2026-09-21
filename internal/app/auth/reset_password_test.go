package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// A reset link is bound to the password it was requested against: once any
// path writes a new one, every link issued at or before that write is dead,
// whatever its own expiry says.
func TestResetLinkPredatesPassword(t *testing.T) {
	changed := time.Date(2026, 9, 21, 12, 0, 0, 500_000_000, time.UTC)
	at := func(t time.Time) *jwt.NumericDate { return jwt.NewNumericDate(t) }

	cases := []struct {
		name      string
		issuedAt  *jwt.NumericDate
		changedAt *time.Time
		stale     bool
	}{
		{"never changed", at(changed.Add(-time.Hour)), nil, false},
		{"issued before the change", at(changed.Add(-time.Hour)), &changed, true},
		{"issued after the change", at(changed.Add(time.Hour)), &changed, false},
		// iat is whole seconds, so a token minted in the same second as the
		// change cannot prove it came after it.
		{"issued in the same second", at(changed), &changed, true},
		{"issued the next second", at(changed.Add(time.Second)), &changed, false},
		{"no issue time at all", nil, &changed, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resetLinkPredatesPassword(tc.issuedAt, tc.changedAt); got != tc.stale {
				t.Fatalf("stale = %v, want %v", got, tc.stale)
			}
		})
	}
}
