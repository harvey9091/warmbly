package jobrun

import (
	"testing"
	"time"
)

// Nine jobs share the hourly interval, and before the offset existed they all
// ran in the same second forever, which is what exhausted the database's
// connections. They only have to land in different seconds, not evenly.
func TestPhaseOffsetSpreadsJobsThatShareAnInterval(t *testing.T) {
	names := []string{
		"org_transfer_housekeeping", "form_events_retention", "danger_zone",
		"incoming_reply_repair", "warmup_health_sweep", "fleet_rebalance",
		"tracking_dedupe_purge", "suppression_retention", "audit_retention",
	}

	seconds := make(map[int64]string, len(names))
	for _, name := range names {
		offset := phaseOffset(name, time.Hour)
		if offset < 0 || offset >= phaseWindow {
			t.Fatalf("%s: offset %s outside the window", name, offset)
		}
		sec := int64(offset / time.Second)
		if other, clash := seconds[sec]; clash {
			t.Errorf("%s and %s both start at +%ds", name, other, sec)
		}
		seconds[sec] = name
	}
}

// The offset is a place in the queue, not a delay: a restart that reshuffled
// it would let two jobs collide tomorrow that did not collide today.
func TestPhaseOffsetIsStableAcrossRestarts(t *testing.T) {
	first := phaseOffset("danger_zone", time.Hour)
	for i := 0; i < 100; i++ {
		if got := phaseOffset("danger_zone", time.Hour); got != first {
			t.Fatalf("offset moved between calls: %s then %s", first, got)
		}
	}
}

// A job that runs every 30 seconds must not be held for 45, or it would miss
// its own period on every boot.
func TestPhaseOffsetNeverExceedsTheJobsOwnInterval(t *testing.T) {
	for _, interval := range []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, time.Minute, time.Hour} {
		for _, name := range []string{"a", "danger_zone", "incoming_reply_repair", "zzz"} {
			offset := phaseOffset(name, interval)
			if offset >= interval {
				t.Errorf("%s at interval %s: offset %s is not shorter than the interval", name, interval, offset)
			}
			if offset < 0 {
				t.Errorf("%s at interval %s: negative offset %s", name, interval, offset)
			}
		}
	}
}

func TestPhaseOffsetHandlesAnUnsetInterval(t *testing.T) {
	if got := phaseOffset("job", 0); got != 0 {
		t.Fatalf("expected no offset for a zero interval, got %s", got)
	}
}
