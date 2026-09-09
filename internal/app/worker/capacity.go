// Capacity is the math layer that turns a row from worker_capacity_view
// into a placement decision. Kept as plain functions so tests can drive
// every edge case without touching the database.
//
// Why compute Effective in Go instead of pushing it into the view? Two
// reasons:
//
//   1. Tests can override Base/HealthMul/AgeMul independently and verify
//      the floor/ceiling behavior without spinning up Postgres.
//   2. The view exposes the raw inputs (sends_attempted_1h, bounces, etc.)
//      so an operator or future feature can compute a different
//      effective-capacity formula without a migration.

package worker

import (
	"math"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// WorkerCapacityRow is the in-Go representation of one row of
// worker_capacity_view. Loaded by the repository; fed into ComputeCapacity.
type WorkerCapacityRow struct {
	WorkerID         uuid.UUID
	Region           string
	HealthState      models.WorkerHealthState
	LoadScore        float64
	BaseCapacity     float64
	HealthMultiplier float64
	AgeMultiplier    float64
	SendsAttempted1h int64
	SendsSucceeded1h int64
	BouncesHard1h    int64
	BouncesSoft1h    int64
	Complaints1h     int64
	AuthErrors1h     int64
}

// Capacity is the derived placement view of a worker. All fields are
// dimensionless except Effective (mailbox-equivalents) and Load (sum of
// mailbox weights). Utilization is Load/Effective and is the value the
// scheduler sorts on when picking the next worker.
type Capacity struct {
	Base        float64
	HealthMul   float64
	AgeMul      float64
	Effective   float64
	Load        float64
	Utilization float64
}

// ComputeCapacity is the placement math: Base * Health * Age, floored at
// 1, then load is divided through to give a utilization ratio. The floor
// matters because a brand-new worker with zero history would otherwise
// have Effective=0 and never get probed.
func ComputeCapacity(row WorkerCapacityRow) Capacity {
	c := Capacity{
		Base:      row.BaseCapacity,
		HealthMul: clampUnit(row.HealthMultiplier),
		AgeMul:    clampUnit(row.AgeMultiplier),
		Load:      row.LoadScore,
	}
	c.Effective = math.Floor(c.Base * c.HealthMul * c.AgeMul)
	if c.Effective <= 0 {
		c.Effective = 1
	}
	if c.Effective > 0 {
		c.Utilization = c.Load / c.Effective
	}
	return c
}

// clampUnit pins x into [0, 1]. Negatives are surprisingly easy to feed
// in - PostgreSQL's NULLIF/divide-by-zero handling can leak NaN through
// in pathological cases - so we coerce defensively here.
func clampUnit(x float64) float64 {
	if math.IsNaN(x) || x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// MailboxWeight is the per-mailbox load contribution, in cold-mailbox
// equivalents. It is the reason a worker no longer declares an egress
// category: each mailbox states its own cost, so one base capacity covers a
// worker carrying any mix.
//
//   - smtp_imap mailboxes hold a real SMTP and IMAP conversation from the
//     worker's own address, and Exchange Online caps SMTP AUTH at ~3
//     concurrent connections and ~30 msg/min per mailbox. They are the
//     bottleneck. Weight = 1.0, so a worker with Base=16 carries ~16 of them.
//
//   - gmail and outlook mailboxes go through the Google and Microsoft Graph
//     APIs. The provider absorbs the connection cost and the per-mailbox
//     ceiling is its own quota, not ours. Weight = 0.05.
//
//   - Warmup-only assignments are cheapest: warmup volume is small and bursty
//     by design. Weight = 0.4 regardless of provider so warmup-only workers
//     are not crowded out by their own cold-style accounting.
//
// The provider strings are the email_provider enum values as stored. An
// earlier version of this function switched on "gmail-api" and "graph-api",
// which no caller ever produced, so every non-warmup mailbox silently weighed
// 1.0 and the API-backed ones were over-accounted by 20x.
func MailboxWeight(provider string, isWarmup bool) float64 {
	if isWarmup {
		return 0.4
	}
	switch models.InboxProvider(provider) {
	case models.InboxProviderGoogle, models.InboxProviderOutlook:
		return 0.05
	default:
		return 1.0
	}
}
