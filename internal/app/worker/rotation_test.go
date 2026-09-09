package worker

import (
	"testing"
	"time"

	"github.com/warmbly/warmbly/internal/models"
)

func healthyInput() RotationInput {
	return RotationInput{
		WorkerActive: true,
		WorkerLive:   true,
		WorkerHealth: models.WorkerHealthHealthy,
		Residency:    30 * 24 * time.Hour,
	}
}

func TestHealthyWorkerNeverRotates(t *testing.T) {
	urgency, reason := EvaluateRotation(healthyInput())
	if urgency != RotationStay {
		t.Fatalf("a settled mailbox on a healthy worker must stay, got %v (%s)", urgency, reason)
	}
}

func TestDeadOrBlockedWorkerIsImmediate(t *testing.T) {
	cases := map[string]func(RotationInput) RotationInput{
		"inactive": func(in RotationInput) RotationInput { in.WorkerActive = false; return in },
		"no heartbeat": func(in RotationInput) RotationInput {
			in.WorkerLive = false
			return in
		},
		"blocked": func(in RotationInput) RotationInput {
			in.WorkerHealth = models.WorkerHealthBlocked
			return in
		},
		"quarantined": func(in RotationInput) RotationInput {
			in.WorkerHealth = models.WorkerHealthQuarantined
			return in
		},
	}
	for name, mutate := range cases {
		in := mutate(healthyInput())
		in.Residency = time.Minute // fresh: residency must not hold it back
		urgency, reason := EvaluateRotation(in)
		if urgency != RotationImmediate {
			t.Fatalf("%s: expected immediate, got %v", name, urgency)
		}
		if reason == "" {
			t.Fatalf("%s: urgent rotations must carry a reason", name)
		}
		if !MayMove(urgency, in.Residency) {
			t.Fatalf("%s: residency must not block an immediate move", name)
		}
	}
}

func TestThrottledWorkerWaitsOutTheShortResidency(t *testing.T) {
	in := healthyInput()
	in.WorkerHealth = models.WorkerHealthThrottled
	in.Residency = time.Hour

	urgency, _ := EvaluateRotation(in)
	if urgency != RotationElevated {
		t.Fatalf("throttled should be elevated, got %v", urgency)
	}
	if MayMove(urgency, time.Hour) {
		t.Fatal("an elevated move must respect the short residency floor")
	}
	if !MayMove(urgency, RotationElevatedResidency) {
		t.Fatal("an elevated move should be allowed once the short floor passes")
	}
}

func TestHotWorkerIsOpportunisticAndNeedsAMateriallyBetterHome(t *testing.T) {
	in := healthyInput()
	in.WorkerUtilization = RotationHotUtilization + 0.05

	urgency, _ := EvaluateRotation(in)
	if urgency != RotationOpportunistic {
		t.Fatalf("an over-capacity worker should be opportunistic, got %v", urgency)
	}
	if MayMove(urgency, time.Hour) {
		t.Fatal("an opportunistic move must respect the full residency floor")
	}
	if !MayMove(urgency, RotationMinResidency) {
		t.Fatal("an opportunistic move should be allowed after full residency")
	}

	// A marginal improvement is not worth the provider-trust cost.
	if WorthMoving(urgency, 1.0, 1.0+RotationMinScoreGain/2, false) {
		t.Fatal("a marginal score gain must not justify a migration")
	}
	if !WorthMoving(urgency, 1.0, 1.0+RotationMinScoreGain, false) {
		t.Fatal("a material score gain should justify a migration")
	}
}

func TestUrgentMovesTakeAnythingEligible(t *testing.T) {
	// Staying is not an option, so a worse-scoring destination still wins.
	if !WorthMoving(RotationImmediate, 5.0, 0.1, false) {
		t.Fatal("an immediate move must accept any eligible destination")
	}
	if !WorthMoving(RotationElevated, 5.0, 0.1, false) {
		t.Fatal("an elevated move must accept any eligible destination")
	}
}

func TestReservedWorkerDriftRotatesBothWays(t *testing.T) {
	// A stranger has to leave for the reservation to mean anything, and the
	// destination does not have to be better than where it is. Weighing it
	// against the incumbent's stickiness bonus would refuse the move forever.
	stranger := healthyInput()
	stranger.OnSomeoneElsesReservedWorker = true
	urgency, _ := EvaluateRotation(stranger)
	if urgency != RotationElevated {
		t.Fatalf("a mailbox on someone else's reserved worker must be evicted, got %v", urgency)
	}
	if !WorthMoving(urgency, 5.0, 0.1, false) {
		t.Fatal("evicting a stranger must accept any eligible destination")
	}

	// Pulling the owner back is opportunistic, but the placer marks the
	// reserved worker as mandated so the score comparison does not block it.
	owner := healthyInput()
	owner.AwayFromOwnReservedWorker = true
	urgency, _ = EvaluateRotation(owner)
	if urgency != RotationOpportunistic {
		t.Fatalf("a mailbox away from its own reserved worker should be pulled back, got %v", urgency)
	}
	if !WorthMoving(urgency, 5.0, 0.1, true) {
		t.Fatal("the reserved worker is mandated, so the move must not be score-gated")
	}
}

func TestUnknownResidencyIsTreatedAsSettled(t *testing.T) {
	// worker_assigned_at was backfilled at migration time, so a zero value
	// means an old assignment, not a brand new one. Reading it as "brand new"
	// would freeze every pre-migration mailbox in place.
	if !MayMove(RotationOpportunistic, 0) {
		t.Fatal("unknown residency must not block an opportunistic move")
	}
}

// A reserved worker usually scores lower than the incumbent, because the
// incumbent carries the stickiness bonus. Weighing the two would refuse the
// move on every tick and an isolated-egress organization would never arrive on
// the worker it is paying for.
func TestMandatedTargetIgnoresTheScoreComparison(t *testing.T) {
	if WorthMoving(RotationOpportunistic, 5.0, 0.1, false) {
		t.Fatal("an ordinary opportunistic move must not accept a worse target")
	}
	if !WorthMoving(RotationOpportunistic, 5.0, 0.1, true) {
		t.Fatal("an entitlement-chosen target must move regardless of score")
	}
	if WorthMoving(RotationStay, 0, 100, true) {
		t.Fatal("mandated must not override RotationStay")
	}
}

func TestStayNeverMoves(t *testing.T) {
	if MayMove(RotationStay, RotationMinResidency*10) {
		t.Fatal("RotationStay must never permit a move")
	}
	if WorthMoving(RotationStay, 0, 100, false) {
		t.Fatal("RotationStay must never be worth moving")
	}
}
