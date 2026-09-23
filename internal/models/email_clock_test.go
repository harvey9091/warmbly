package models

import "testing"

// A mailbox reads its own hours in its own zone, else the workspace's; empty
// is UTC, and the campaign path never consults this (it follows the campaign).
func TestClockTimezoneFallsBackToTheWorkspace(t *testing.T) {
	for _, tc := range []struct{ own, org, want string }{
		{"", "", ""},
		{"", "America/New_York", "America/New_York"},
		{"Europe/Paris", "America/New_York", "Europe/Paris"},
		{"Europe/Paris", "", "Europe/Paris"},
	} {
		e := Email{Timezone: tc.own, OrgTimezone: tc.org}
		if got := e.ClockTimezone(); got != tc.want {
			t.Errorf("own %q org %q: got %q, want %q", tc.own, tc.org, got, tc.want)
		}
	}
}
