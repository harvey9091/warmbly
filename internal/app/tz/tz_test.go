package tz

import "testing"

// The curated list is what the picker offers; a browser reports whatever zone
// the machine is in, and that has to be accepted too.
func TestValidAcceptsAnyIANAZone(t *testing.T) {
	for _, name := range []string{"America/New_York", "Europe/Budapest", "America/Detroit", "UTC"} {
		if !Valid(name) {
			t.Errorf("Valid(%q) = false", name)
		}
	}
	for _, name := range []string{"", "EST", "Mars/Olympus", "America/New York", "Local"} {
		if Valid(name) {
			t.Errorf("Valid(%q) = true", name)
		}
	}
}
