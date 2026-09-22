package warmup

import (
	"testing"
	"time"

	"github.com/warmbly/warmbly/internal/models"
)

func TestEvaluateMetricsSpamThresholds(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		rate        float64
		wantState   models.WarmupHealthState
		wantBlocked time.Duration
	}{
		{name: "watch", rate: 10, wantState: models.WarmupHealthWatch},
		{name: "quarantine", rate: 20, wantState: models.WarmupHealthQuarantined, wantBlocked: warmupQuarantineDuration},
		{name: "block", rate: 40, wantState: models.WarmupHealthBlocked, wantBlocked: warmupBlockDuration},
		{name: "catastrophic", rate: 80, wantState: models.WarmupHealthBlocked, wantBlocked: warmupCatastrophicBlock},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := evaluateMetrics(&models.WarmupHealthMetrics{
				SentLast7d:        20,
				SpamPlacementRate: tc.rate,
			}, now)

			if decision.State != tc.wantState {
				t.Fatalf("expected %s, got %s", tc.wantState, decision.State)
			}
			if tc.wantBlocked == 0 {
				if decision.BlockedUntil != nil {
					t.Fatalf("expected no block, got %#v", decision.BlockedUntil)
				}
				return
			}
			if decision.BlockedUntil == nil || !decision.BlockedUntil.Equal(now.Add(tc.wantBlocked)) {
				t.Fatalf("expected blocked until %s, got %#v", now.Add(tc.wantBlocked), decision.BlockedUntil)
			}
		})
	}
}

func TestEvaluateMetricsThrottleBand(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        25,
		SpamPlacementRate: 16.0, // between 15% (throttle) and 20% (quarantine)
	}, now)

	if decision.State != models.WarmupHealthThrottled {
		t.Fatalf("expected throttled, got %s", decision.State)
	}
	if decision.BlockedUntil == nil {
		t.Fatal("throttled should have blocked_until set")
	}
	if !decision.BlockedUntil.Equal(now.Add(warmupThrottleDuration)) {
		t.Fatalf("expected 3-day throttle, got %v", decision.BlockedUntil.Sub(now))
	}
}

func TestEvaluateMetricsComplaintRateWatch(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        5,
		DeliveredLast30d:  200,
		ComplaintsLast30d: 1,
		ComplaintRate:     0.05, // between 0.03% (watch) and 0.10% (quarantine)
	}, now)

	if decision.State != models.WarmupHealthWatch {
		t.Fatalf("expected watch from complaint rate, got %s", decision.State)
	}
}

func TestEvaluateMetricsComplaintRateQuarantine(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        5,
		DeliveredLast30d:  200,
		ComplaintsLast30d: 2,
		ComplaintRate:     0.15, // > 0.10% quarantine
	}, now)

	if decision.State != models.WarmupHealthQuarantined {
		t.Fatalf("expected quarantined from complaint rate, got %s", decision.State)
	}
}

func TestEvaluateMetricsComplaintRateBlock(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        5,
		DeliveredLast30d:  200,
		ComplaintsLast30d: 10,
		ComplaintRate:     0.5, // > 0.30% block
	}, now)

	if decision.State != models.WarmupHealthBlocked {
		t.Fatalf("expected blocked from complaint rate, got %s", decision.State)
	}
}

func TestEvaluateMetricsBounceRateQuarantine(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:       5,
		DeliveredLast30d: 200,
		BouncesLast30d:   12,
		BounceRate:       6.0, // > 5% quarantine
	}, now)

	if decision.State != models.WarmupHealthQuarantined {
		t.Fatalf("expected quarantined from bounce rate, got %s", decision.State)
	}
}

func TestEvaluateMetricsBounceRateBlock(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:       5,
		DeliveredLast30d: 200,
		BouncesLast30d:   25,
		BounceRate:       12.5, // > 10% block
	}, now)

	if decision.State != models.WarmupHealthBlocked {
		t.Fatalf("expected blocked from bounce rate, got %s", decision.State)
	}
}

func TestEvaluateMetricsComplaintBelowSampleIgnored(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        5,
		DeliveredLast30d:  50, // below 100 minimum
		ComplaintsLast30d: 5,
		ComplaintRate:     10.0,
	}, now)

	if decision.State == models.WarmupHealthQuarantined || decision.State == models.WarmupHealthBlocked {
		t.Fatalf("should not quarantine/block with insufficient sample, got %s", decision.State)
	}
}

func TestEvaluateMetricsIgnoresSmallSamples(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)

	decision := evaluateMetrics(&models.WarmupHealthMetrics{
		SentLast7d:        19,
		SpamPlacementRate: 100,
	}, now)

	if decision.State != models.WarmupHealthHealthy {
		t.Fatalf("expected healthy for undersampled account, got %s", decision.State)
	}
	if decision.BlockedUntil != nil {
		t.Fatalf("expected no block, got %#v", decision.BlockedUntil)
	}
}

// Harm to received warmup mail climbs a ladder rather than blocking on the
// first event: one deletion is housekeeping until proven otherwise (#635).
func TestEvaluateMetricsTamperingLadder(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		deletions   int
		spamFlags   int
		wantState   models.WarmupHealthState
		wantBlocked time.Duration
	}{
		{"nothing", 0, 0, models.WarmupHealthHealthy, 0},
		{"one deletion warns", 1, 0, models.WarmupHealthWatch, 0},
		{"two deletions pause", 2, 0, models.WarmupHealthQuarantined, warmupQuarantineDuration},
		{"one spam flag pauses", 0, 1, models.WarmupHealthQuarantined, warmupQuarantineDuration},
		{"four deletions block", 4, 0, models.WarmupHealthBlocked, warmupBlockDuration},
		{"two spam flags block", 0, 2, models.WarmupHealthBlocked, warmupBlockDuration},
		{"a flag and two deletions block", 2, 1, models.WarmupHealthBlocked, warmupBlockDuration},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := evaluateMetrics(&models.WarmupHealthMetrics{DeletionsLast7d: tc.deletions, SpamFlagsLast7d: tc.spamFlags}, now)
			if decision.State != tc.wantState {
				t.Fatalf("state = %s, want %s (reason %q)", decision.State, tc.wantState, decision.Reason)
			}
			if tc.wantBlocked == 0 {
				if decision.BlockedUntil != nil {
					t.Fatalf("a %s carries a term: %v", tc.wantState, decision.BlockedUntil)
				}
				return
			}
			if decision.BlockedUntil == nil || !decision.BlockedUntil.Equal(now.Add(tc.wantBlocked)) {
				t.Fatalf("blocked until %v, want %s", decision.BlockedUntil, now.Add(tc.wantBlocked))
			}
			if decision.Reason == "" {
				t.Fatal("a tampering decision carries no reason for the owner")
			}
		})
	}
}

// A tampering block never requires review: it lapses like every other band,
// so an accidental run of deletions is not permanent.
func TestEvaluateMetricsTamperingBlockLapses(t *testing.T) {
	decision := evaluateMetrics(&models.WarmupHealthMetrics{DeletionsLast7d: 6}, time.Now())
	if decision.State != models.WarmupHealthBlocked || decision.BlockedUntil == nil {
		t.Fatalf("decision = %+v, want a block with a term", decision)
	}
}

// The rate bands and the tampering band are judged apart and the more severe
// finding is kept, whichever side it comes from.
func TestEvaluateMetricsTamperingCombinesWithRates(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	block := now.Add(warmupBlockDuration)
	quarantine := now.Add(warmupQuarantineDuration)
	cases := []struct {
		name    string
		metrics models.WarmupHealthMetrics
		want    models.WarmupHealthState
		until   *time.Time
	}{
		{"one deletion does not mask a placement block", models.WarmupHealthMetrics{DeletionsLast7d: 1, SentLast7d: 20, SpamPlacementRate: 40}, models.WarmupHealthBlocked, &block},
		{"a placement watch does not mask a tampering quarantine", models.WarmupHealthMetrics{DeletionsLast7d: 2, SentLast7d: 20, SpamPlacementRate: 10}, models.WarmupHealthQuarantined, &quarantine},
		{"a complaint-rate quarantine does not mask a tampering block", models.WarmupHealthMetrics{DeletionsLast7d: 4, DeliveredLast30d: 100, ComplaintRate: complaintRateQuarantinePct}, models.WarmupHealthBlocked, &block},
		{"a bounce-rate quarantine does not mask a tampering block", models.WarmupHealthMetrics{SpamFlagsLast7d: 2, DeliveredLast30d: 100, BounceRate: bounceRateQuarantinePct}, models.WarmupHealthBlocked, &block},
		{"a warmup-complaint quarantine does not mask a tampering block", models.WarmupHealthMetrics{DeletionsLast7d: 4, SentLast7d: 20, WarmupComplaintRate: warmupComplaintQuarantinePct}, models.WarmupHealthBlocked, &block},
		{"a placement throttle does not mask a tampering quarantine", models.WarmupHealthMetrics{DeletionsLast7d: 2, SentLast7d: 20, SpamPlacementRate: spamPlacementThrottlePct}, models.WarmupHealthQuarantined, &quarantine},
		{"a longer rate block outlasts a tampering block", models.WarmupHealthMetrics{DeletionsLast7d: 4, SentLast7d: 20, SpamPlacementRate: 80}, models.WarmupHealthBlocked, func() *time.Time { u := now.Add(warmupCatastrophicBlock); return &u }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.metrics
			decision := evaluateMetrics(&m, now)
			if decision.State != tc.want {
				t.Fatalf("state = %s, want %s (reason %q)", decision.State, tc.want, decision.Reason)
			}
			if (decision.BlockedUntil == nil) != (tc.until == nil) {
				t.Fatalf("blocked until %v, want %v", decision.BlockedUntil, tc.until)
			}
			if tc.until != nil && !decision.BlockedUntil.Equal(*tc.until) {
				t.Fatalf("blocked until %v, want %s", decision.BlockedUntil, tc.until)
			}
		})
	}
}
