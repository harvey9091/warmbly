package jobs

import (
	"testing"
	"time"

	"github.com/warmbly/warmbly/internal/app/instancesettings"
	"github.com/warmbly/warmbly/internal/repository"
)

func strp(s string) *string { return &s }

func TestIsInstantUsesTheDispatchClock(t *testing.T) {
	sent := time.Now()
	window := time.Minute
	if !isInstant(&sent, sent.Add(3*time.Second), window) {
		t.Fatal("three seconds after dispatch is a machine")
	}
	if isInstant(&sent, sent.Add(90*time.Second), window) {
		t.Fatal("past the window is a person")
	}
	if isInstant(nil, sent, window) {
		t.Fatal("an unknown dispatch time must never count as instant")
	}
	// The window is a half-open interval, so the boundary itself is already
	// out. Without this the two windows would overlap by a second.
	if isInstant(&sent, sent.Add(window), window) {
		t.Fatal("the boundary is outside the window")
	}
	// A stamp before the dispatch means a skewed clock somewhere, not a
	// person who read the mail first. Treating a negative gap as "inside the
	// window" would mark every such event automated, so it is neither.
	if isInstant(&sent, sent.Add(-time.Hour), window) {
		t.Fatal("an event stamped before dispatch is not instant")
	}
}

// The window is a deployment property, not a constant: the clock starts when
// the send is handed to the worker, so it has to cover provider queueing and
// transit before the recipient's gateway has even seen the message.
func TestMachineWindowsAreOperatorEditable(t *testing.T) {
	sent := time.Now()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	at := sent.Add(90 * time.Second)

	if m, _ := classifyOpen(chrome, nil, &sent, at, instancesettings.DefaultTracking().OpenWindow()); m {
		t.Fatal("ninety seconds is past the shipped open window")
	}
	widened := instancesettings.Tracking{MachineWindowOpenSeconds: 120}
	widened.Normalize()
	if m, r := classifyOpen(chrome, nil, &sent, at, widened.OpenWindow()); !m || r != repository.EmailOpenReasonInstant {
		t.Fatalf("a widened window catches it, got %v %q", m, r)
	}
}

// Opens and clicks are tuned separately because the two mistakes cost
// different things: a misjudged open loses a metric, a misjudged click loses
// the automation behind an interested lead.
func TestOpenAndClickWindowsAreIndependent(t *testing.T) {
	sent := time.Now()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	windows := instancesettings.DefaultTracking()
	at := sent.Add(45 * time.Second)

	if m, r := classifyOpen(chrome, nil, &sent, at, windows.OpenWindow()); !m || r != repository.EmailOpenReasonInstant {
		t.Fatalf("forty-five seconds is inside the shipped open window, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, at, windows.ClickWindow()); m || r != "" {
		t.Fatalf("the same moment is outside the shipped click window, got %v %q", m, r)
	}
}

func TestClassifyClick(t *testing.T) {
	sent := time.Now()
	clickWindow := instancesettings.DefaultTracking().ClickWindow()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	if m, r := classifyClick(nil, nil, &sent, sent.Add(time.Minute), clickWindow); !m || r != repository.LinkClickReasonPrefetch {
		t.Fatalf("no user agent = prefetch, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, sent.Add(2*time.Second), clickWindow); !m || r != repository.LinkClickReasonInstant {
		t.Fatalf("a browser UA two seconds after dispatch = instant, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, sent.Add(time.Minute), clickWindow); m || r != "" {
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
	windows := instancesettings.DefaultTracking()
	openWindow, clickWindow := windows.OpenWindow(), windows.ClickWindow()
	chrome := strp("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	net := strp("microsoft-365-protection")

	if m, r := classifyClick(chrome, net, &sent, late, clickWindow); !m || r != repository.LinkClickReasonScanner {
		t.Fatalf("a click from a scanner network = scanner, got %v %q", m, r)
	}
	if m, r := classifyOpen(chrome, net, &sent, late, openWindow); !m || r != repository.EmailOpenReasonScanner {
		t.Fatalf("an open from a scanner network = scanner, got %v %q", m, r)
	}
	// An empty label is the same as none: the edge recognised nothing, and a
	// blank string must not silently condemn every event that carries it.
	if m, r := classifyOpen(chrome, strp("  "), &sent, late, openWindow); m || r != "" {
		t.Fatalf("a blank scanner label is not a verdict, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, nil, &sent, late, clickWindow); m || r != "" {
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
