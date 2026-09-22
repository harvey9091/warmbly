// Capacity turns worker_capacity_view rows into placement inputs.

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

// Capacity is the worker's assigned-mailbox load against its local target.
type Capacity struct {
	Base        float64
	HealthMul   float64
	AgeMul      float64
	Load        float64
	Utilization float64
	Target      float64
}

// ComputeCapacity keeps node age in placement scoring without shrinking its target.
func ComputeCapacity(row WorkerCapacityRow) Capacity {
	c := Capacity{
		Base:      row.BaseCapacity,
		HealthMul: clampUnit(row.HealthMultiplier),
		AgeMul:    clampUnit(row.AgeMultiplier),
		Load:      row.LoadScore,
	}
	c.Target, c.Utilization = models.WorkerOperationalCapacity(c.Load, c.Base, c.HealthMul)
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

// MailboxWeight counts every assigned mailbox equally.
func MailboxWeight(_ string, _ bool) float64 {
	return 1
}
