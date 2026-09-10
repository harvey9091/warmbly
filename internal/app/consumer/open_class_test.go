package jobs

import (
	"testing"
	"time"

	"github.com/warmbly/warmbly/internal/repository"
)

func strp(s string) *string { return &s }

func TestIsInstantUsesTheDispatchClock(t *testing.T) {
	sent := time.Now()
	if !isInstant(&sent, sent.Add(3*time.Second)) {
		t.Fatal("three seconds after dispatch is a machine")
	}
	if isInstant(&sent, sent.Add(45*time.Second)) {
		t.Fatal("forty-five seconds after dispatch can be a person")
	}
	if isInstant(nil, sent) {
		t.Fatal("an unknown dispatch time must never count as instant")
	}
}

func TestClassifyClick(t *testing.T) {
	sent := time.Now()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	if m, r := classifyClick(nil, nil, &sent, sent.Add(time.Minute)); !m || r != repository.LinkClickReasonPrefetch {
		t.Fatalf("no user agent = prefetch, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, sent.Add(2*time.Second)); !m || r != repository.LinkClickReasonInstant {
		t.Fatalf("a browser UA two seconds after dispatch = instant, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, sent.Add(time.Minute)); m || r != "" {
		t.Fatalf("a browser a minute later is a person, got %v %q", m, r)
	}
}

// A security gateway walks a message with an ordinary browser's user agent
// and can do it long after delivery, so neither the UA rules nor the machine
// window sees it. The edge's verdict on the source network is what does, and
// it outranks both: a Chrome UA from a mail-filtering network is still a scan.
func TestClassifyScannerSourceOutranksTheUserAgent(t *testing.T) {
	sent := time.Now()
	late := sent.Add(time.Hour)
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	net := strp("microsoft-365-protection")

	if m, r := classifyClick(chrome, net, &sent, late); !m || r != repository.LinkClickReasonScanner {
		t.Fatalf("a click from a scanner network = scanner, got %v %q", m, r)
	}
	if m, r := classifyOpen(chrome, net, &sent, late); !m || r != repository.EmailOpenReasonScanner {
		t.Fatalf("an open from a scanner network = scanner, got %v %q", m, r)
	}
	// An empty label is the same as none: the edge recognised nothing, and a
	// blank string must not silently condemn every event that carries it.
	if m, r := classifyOpen(chrome, strp("  "), &sent, late); m || r != "" {
		t.Fatalf("a blank scanner label is not a verdict, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, late); m || r != "" {
		t.Fatalf("no scanner label leaves the click a person's, got %v %q", m, r)
	}
}

func TestEventTimeFallsBackToNow(t *testing.T) {
	stamp := "2026-09-03T10:00:00Z"
	if got := eventTime(stamp); !got.Equal(time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected parse: %v", got)
	}
	if d := time.Since(eventTime("garbage")); d < 0 || d > time.Minute {
		t.Fatalf("unreadable stamp should fall back to now, got %v ago", d)
	}
}
