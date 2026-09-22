package fleet

import (
	"math"
	"testing"

	"github.com/warmbly/warmbly/internal/repository"
)

func TestFleetUtilizationUsesOperationalTargetInsteadOfNodeAge(t *testing.T) {
	rows := []repository.WorkerCapacityRowDB{{
		LoadScore:        88,
		BaseCapacity:     100,
		HealthMultiplier: 1,
		AgeMultiplier:    1.0 / 72,
	}}

	load, capacity, utilization := fleetUtilization(rows)
	if load != 88 || capacity != 100 {
		t.Fatalf("load/capacity = %.2f/%.2f, want 88/100", load, capacity)
	}
	if math.Abs(utilization-0.88) > 1e-9 {
		t.Fatalf("utilization = %.4f, want 0.88", utilization)
	}
}
