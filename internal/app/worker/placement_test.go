package worker

import (
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// candidate builds a healthy candidate with the given capacity numbers.
func candidate(id uuid.UUID, effective, load float64) PlacementCandidate {
	c := PlacementCandidate{
		WorkerID: id,
		Health:   models.WorkerHealthHealthy,
		Capacity: Capacity{Effective: effective, Load: load},
	}
	if effective > 0 {
		c.Capacity.Utilization = load / effective
	}
	return c
}

func TestEligibleRequiresHealthAndHeadroom(t *testing.T) {
	id := uuid.New()
	req := PlacementRequest{Weight: 1.0}

	if c := candidate(id, 16, 4); !c.Eligible(req) {
		t.Fatal("healthy worker with headroom should be eligible")
	}
	if c := candidate(id, 16, 15.5); c.Eligible(req) {
		t.Fatal("worker with less headroom than the mailbox weight should not be eligible")
	}

	for _, state := range []models.WorkerHealthState{
		models.WorkerHealthThrottled,
		models.WorkerHealthQuarantined,
		models.WorkerHealthBlocked,
	} {
		c := candidate(id, 16, 0)
		c.Health = state
		if c.Eligible(req) {
			t.Fatalf("state %s should not accept new mailboxes", state)
		}
	}

	// watch still accepts: it is a warning, not a stop.
	c := candidate(id, 16, 0)
	c.Health = models.WorkerHealthWatch
	if !c.Eligible(req) {
		t.Fatal("watch state should still accept placements")
	}
}

func TestIncumbentOutranksMarginallyEmptierWorker(t *testing.T) {
	incumbent := uuid.New()
	empty := uuid.New()

	// The incumbent is busier, but staying put avoids a client-IP change.
	cands := []PlacementCandidate{
		candidate(incumbent, 16, 12),
		candidate(empty, 16, 0),
	}
	got := SelectPlacement(cands, PlacementRequest{Weight: 1.0, CurrentWorkerID: &incumbent})
	if got == nil || got.WorkerID != incumbent {
		t.Fatalf("expected incumbent to win, got %v", got)
	}
}

func TestIncumbentLosesWhenItHasNoHeadroom(t *testing.T) {
	incumbent := uuid.New()
	other := uuid.New()

	cands := []PlacementCandidate{
		candidate(incumbent, 16, 16),
		candidate(other, 16, 8),
	}
	got := SelectPlacement(cands, PlacementRequest{Weight: 1.0, CurrentWorkerID: &incumbent})
	if got == nil || got.WorkerID != other {
		t.Fatal("a full incumbent must not win on stickiness alone")
	}
}

func TestBlastRadiusSpreadsAnOrgAcrossWorkers(t *testing.T) {
	crowded := uuid.New()
	fresh := uuid.New()

	// Both equally loaded overall, but the org already has all 10 of its
	// mailboxes on `crowded`.
	a := candidate(crowded, 16, 8)
	a.OrgMailboxesHere = 10
	b := candidate(fresh, 16, 8)

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{
		Weight:            1.0,
		OrgMailboxesTotal: 10,
	})
	if got == nil || got.WorkerID != fresh {
		t.Fatal("placement should spread an org rather than concentrate it")
	}
}

func TestProviderCrowdingSteersAwayFromOneAddress(t *testing.T) {
	crowded := uuid.New()
	fresh := uuid.New()

	a := candidate(crowded, 16, 8)
	a.ProviderMailboxesHere = int(providerSoftCap)
	b := candidate(fresh, 16, 8)

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{Weight: 1.0})
	if got == nil || got.WorkerID != fresh {
		t.Fatal("many mailboxes of one provider on one address should push placement elsewhere")
	}
}

func TestIsolatedEgressPrefersAWorkerWithoutForeignTenants(t *testing.T) {
	shared := uuid.New()
	ours := uuid.New()

	a := candidate(shared, 16, 4)
	a.TotalMailboxes = 10
	a.OrgMailboxesHere = 1

	b := candidate(ours, 16, 6)
	b.TotalMailboxes = 6
	b.OrgMailboxesHere = 6

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{
		Weight:         1.0,
		IsolatedEgress: true,
	})
	if got == nil || got.WorkerID != ours {
		t.Fatal("an isolated-egress org should avoid a worker full of other tenants")
	}
}

func TestRegionMatchIsAPreferenceNotARequirement(t *testing.T) {
	match := uuid.New()
	other := uuid.New()

	a := candidate(match, 16, 8)
	a.Region = "eu-central"
	b := candidate(other, 16, 8)
	b.Region = "us-east"

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{Weight: 1.0, Region: "eu-central"})
	if got == nil || got.WorkerID != match {
		t.Fatal("matching region should win between otherwise equal workers")
	}

	// With no region on the request, neither is penalised and the fleet still places.
	if got := SelectPlacement([]PlacementCandidate{b}, PlacementRequest{Weight: 1.0}); got == nil {
		t.Fatal("an unknown region must never make a worker ineligible")
	}
}

func TestSelectPlacementReturnsNilWhenNothingFits(t *testing.T) {
	full := candidate(uuid.New(), 16, 16)
	if got := SelectPlacement([]PlacementCandidate{full}, PlacementRequest{Weight: 1.0}); got != nil {
		t.Fatal("expected no placement when every worker is full")
	}
	if got := SelectPlacement(nil, PlacementRequest{Weight: 1.0}); got != nil {
		t.Fatal("expected no placement from an empty fleet")
	}
}

func TestSelectPlacementIsDeterministicOnTies(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	cands := []PlacementCandidate{candidate(a, 16, 8), candidate(b, 16, 8)}

	first := SelectPlacement(cands, PlacementRequest{Weight: 1.0})
	// Same inputs, reversed order: the winner must not change.
	second := SelectPlacement([]PlacementCandidate{cands[1], cands[0]}, PlacementRequest{Weight: 1.0})
	if first == nil || second == nil || first.WorkerID != second.WorkerID {
		t.Fatal("tie-breaking must not depend on candidate order")
	}
}
