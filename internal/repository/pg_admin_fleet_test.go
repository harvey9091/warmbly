package repository

import (
	"math"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func TestAdminCapacityUsesPlacementTargetInsteadOfNodeAge(t *testing.T) {
	capacity, utilization := models.WorkerOperationalCapacity(88, 100, 1)

	if capacity != 100 {
		t.Fatalf("capacity = %.2f, want placement target 100", capacity)
	}
	if math.Abs(utilization-0.88) > 1e-9 {
		t.Fatalf("utilization = %.4f, want 0.88", utilization)
	}
}
