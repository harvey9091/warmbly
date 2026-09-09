// Rotation decides WHEN a mailbox is allowed to change workers. Placement
// (placement.go) decides where it would go; this decides whether it should go
// at all.
//
// The two are separate because the answer is usually no. Every migration
// changes the client IP the mailbox's provider sees, and providers treat a
// moving sign-in location as a risk signal: Google challenges the login,
// Microsoft and Google both throttle authentication per address. A mailbox
// that stays on one worker for months is in the best possible state, so the
// bar for moving one is deliberately high and rises with how little the move
// buys.

package worker

import (
	"time"

	"github.com/warmbly/warmbly/internal/models"
)

// Rotation thresholds.
const (
	// RotationMinResidency is how long a mailbox stays put before an
	// opportunistic move (packing, isolation) may touch it.
	RotationMinResidency = 72 * time.Hour

	// RotationElevatedResidency is the shorter floor that applies when the
	// current worker is actively degrading. Still non-zero: a worker that
	// dips into throttled and recovers within the hour should not have
	// evacuated its mailboxes.
	RotationElevatedResidency = 6 * time.Hour

	// RotationHotUtilization is the utilization above which a worker is
	// considered worth draining for packing reasons alone.
	RotationHotUtilization = 0.85

	// RotationMinScoreGain is how much better the target must score than the
	// incumbent before an opportunistic move is worth its provider-trust
	// cost. It sits below weightIncumbent so a genuinely better home can
	// still win, but noise cannot.
	RotationMinScoreGain = 0.5
)

// RotationUrgency is how badly a mailbox needs to leave its current worker.
type RotationUrgency int

const (
	// RotationStay means the mailbox is where it should be.
	RotationStay RotationUrgency = iota
	// RotationOpportunistic is a move worth making only if a materially
	// better home exists and the mailbox has served its full residency.
	RotationOpportunistic
	// RotationElevated is a degrading worker: move on the shorter residency,
	// and take any eligible home rather than holding out for a better score.
	RotationElevated
	// RotationImmediate is a worker that cannot do the work at all. Residency
	// and score gain are both ignored; anywhere eligible beats staying.
	RotationImmediate
)

// RotationInput is the live state of one mailbox and the worker under it.
type RotationInput struct {
	WorkerActive      bool
	WorkerLive        bool // heartbeating inside the liveness window
	WorkerHealth      models.WorkerHealthState
	WorkerUtilization float64

	// Residency is how long the mailbox has been on this worker. A zero
	// value (no worker_assigned_at recorded) is treated as "long enough",
	// because the column was backfilled at migration time and a NULL there
	// means an old assignment, not a fresh one.
	Residency time.Duration

	// OnSomeoneElsesReservedWorker is true when this mailbox sits on a worker
	// another organization has reserved. Those have to leave, or the
	// reservation means nothing.
	OnSomeoneElsesReservedWorker bool
	// AwayFromOwnReservedWorker is true when this mailbox's own organization
	// has a reserved worker and the mailbox is not on it.
	AwayFromOwnReservedWorker bool
}

// EvaluateRotation returns how urgently the mailbox should move and a reason
// string suitable for the decision log. The reason is always populated when
// the urgency is not RotationStay.
func EvaluateRotation(in RotationInput) (RotationUrgency, string) {
	// The worker cannot execute anything: a command queued for it is never
	// run and never answered, so this is not a deliverability trade-off.
	if !in.WorkerActive {
		return RotationImmediate, "worker is inactive"
	}
	if !in.WorkerLive {
		return RotationImmediate, "worker stopped heartbeating"
	}
	switch in.WorkerHealth {
	case models.WorkerHealthBlocked:
		return RotationImmediate, "worker is blocked"
	case models.WorkerHealthQuarantined:
		return RotationImmediate, "worker is quarantined"
	case models.WorkerHealthThrottled:
		return RotationElevated, "worker is throttled"
	}

	if in.OnSomeoneElsesReservedWorker {
		// Elevated, not opportunistic: this mailbox has to leave for the
		// reservation to mean anything, and an opportunistic move would be
		// weighed against the incumbent's stickiness bonus and refused every
		// tick, so the stranger would never go and the row would re-enter the
		// scan budget forever.
		return RotationElevated, "worker is reserved for another organization"
	}
	if in.AwayFromOwnReservedWorker {
		return RotationOpportunistic, "organization has a reserved worker elsewhere"
	}

	if in.WorkerUtilization > RotationHotUtilization {
		return RotationOpportunistic, "worker over capacity"
	}

	return RotationStay, ""
}

// MayMove applies the residency floor for an urgency level. Splitting this out
// keeps the "should it move" question separable from "is it allowed to yet",
// which is the part that stops a flapping worker from evacuating twice.
func MayMove(urgency RotationUrgency, residency time.Duration) bool {
	switch urgency {
	case RotationImmediate:
		return true
	case RotationElevated:
		return residency == 0 || residency >= RotationElevatedResidency
	case RotationOpportunistic:
		return residency == 0 || residency >= RotationMinResidency
	default:
		return false
	}
}

// WorthMoving decides whether a chosen target justifies the move.
//
// Urgent moves take anything eligible, because staying is not an option.
// Opportunistic moves have to clear RotationMinScoreGain on top of the
// incumbent's own stickiness bonus, which is what keeps the fleet from
// churning.
//
// A mandated target skips the comparison entirely. It was chosen by an
// entitlement rather than by scoring, and it usually scores lower than the
// incumbent precisely because the incumbent is the incumbent; weighing the two
// would refuse the move on every tick and the mailbox would never arrive.
func WorthMoving(urgency RotationUrgency, incumbentScore, targetScore float64, mandated bool) bool {
	switch urgency {
	case RotationImmediate, RotationElevated:
		return true
	case RotationOpportunistic:
		return mandated || targetScore-incumbentScore >= RotationMinScoreGain
	default:
		return false
	}
}
