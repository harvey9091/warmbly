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

	if m, r := classifyClick(nil, &sent, sent.Add(time.Minute)); !m || r != repository.LinkClickReasonPrefetch {
		t.Fatalf("no user agent = prefetch, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, &sent, sent.Add(2*time.Second)); !m || r != repository.LinkClickReasonInstant {
		t.Fatalf("a browser UA two seconds after dispatch = instant, got %v %q", m, r)
	}
	if m, r := classifyClick(chrome, &sent, sent.Add(time.Minute)); m || r != "" {
		t.Fatalf("a browser a minute later is a person, got %v %q", m, r)
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
