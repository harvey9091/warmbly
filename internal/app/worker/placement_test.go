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
		// AgeMul 1 keeps the youth term out of the way unless a test sets it.
		Capacity: Capacity{Target: effective, Load: load, AgeMul: 1},
	}
	if effective > 0 {
		c.Capacity.Utilization = load / effective
	}
	return c
}

func TestEligibleRequiresHealthOnly(t *testing.T) {
	id := uuid.New()
	req := PlacementRequest{Weight: 1.0}

	if c := candidate(id, 16, 4); !c.Eligible(req) {
		t.Fatal("healthy worker with headroom should be eligible")
	}
	// Capacity is a preference, not a fence: an over-target worker is still a
	// legal home, it just scores badly. See TestOverTargetWorkerIsLastResort.
	if c := candidate(id, 16, 40); !c.Eligible(req) {
		t.Fatal("being over the capacity target must not refuse a placement")
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

func TestRebalanceCanCorrectExistingConcentration(t *testing.T) {
	incumbent, other := uuid.New(), uuid.New()
	a := candidate(incumbent, 100, 32.75)
	a.OrgMailboxesHere = 88
	a.ProviderMailboxesHere = 70
	b := candidate(other, 100, 0)

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{
		Weight:            0.4,
		CurrentWorkerID:   &incumbent,
		IgnoreIncumbency:  true,
		OrgMailboxesTotal: 88,
	})
	if got == nil || got.WorkerID != other {
		t.Fatal("an existing concentration review should choose the worker with headroom")
	}
}

func TestConcentrationMoveUsesFreshCandidateCounts(t *testing.T) {
	result := &PlacementResult{
		CountsKnown:                  true,
		IncumbentOrgMailboxes:        10,
		IncumbentProviderMailboxes:   8,
		DestinationOrgMailboxes:      9,
		DestinationProviderMailboxes: 8,
	}
	if !result.RelievesConcentration(true, true) {
		t.Fatal("a destination that reduces one concentration without worsening the other should be accepted")
	}

	result.DestinationProviderMailboxes = 9
	if result.RelievesConcentration(true, true) {
		t.Fatal("a destination that worsens provider concentration should be refused")
	}

	result.CountsKnown = false
	if result.RelievesConcentration(true, false) {
		t.Fatal("fallback placement without fresh neighbour counts must not drive a concentration move")
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

func TestSelectPlacementReturnsNilOnlyWhenNothingIsHealthy(t *testing.T) {
	sick := candidate(uuid.New(), 16, 0)
	sick.Health = models.WorkerHealthQuarantined
	if got := SelectPlacement([]PlacementCandidate{sick}, PlacementRequest{Weight: 1.0}); got != nil {
		t.Fatal("expected no placement when every worker is unhealthy")
	}
	if got := SelectPlacement(nil, PlacementRequest{Weight: 1.0}); got != nil {
		t.Fatal("expected no placement from an empty fleet")
	}

	// A full fleet is not an empty one. Returning nil here used to drop
	// assignment into selectFallback, which ignores every preference term.
	full := candidate(uuid.New(), 16, 16)
	if got := SelectPlacement([]PlacementCandidate{full}, PlacementRequest{Weight: 1.0}); got == nil {
		t.Fatal("a full but healthy fleet must still place the mailbox")
	}
}

func TestOverTargetWorkerIsLastResort(t *testing.T) {
	over, room := uuid.New(), uuid.New()

	// The over-target worker is also the incumbent and matches the region, so
	// it collects every bonus available. It must still lose to spare capacity.
	a := candidate(over, 16, 18)
	a.Region = "eu"
	b := candidate(room, 16, 15)

	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{
		Weight: 1.0, CurrentWorkerID: &over, Region: "eu",
	})
	if got == nil || got.WorkerID != room {
		t.Fatal("a worker with room must beat an over-target one holding every bonus")
	}
}

func TestAmongOverloadedWorkersTheLeastOverloadedWins(t *testing.T) {
	bad, worse := uuid.New(), uuid.New()

	got := SelectPlacement([]PlacementCandidate{
		candidate(worse, 16, 40),
		candidate(bad, 16, 18),
	}, PlacementRequest{Weight: 1.0})
	if got == nil || got.WorkerID != bad {
		t.Fatal("with no room anywhere, the least-overloaded worker should win")
	}
}

func TestMailboxWeightCountsAgainstTheTarget(t *testing.T) {
	id := uuid.New()
	c := candidate(id, 16, 15)

	gmail := c.Projected(PlacementRequest{Weight: MailboxWeight("gmail", false)})
	smtp := c.Projected(PlacementRequest{Weight: MailboxWeight("smtp_imap", false)})
	if gmail != 1 || smtp != 1 {
		t.Fatalf("every assigned mailbox must count once: gmail=%.3f smtp=%.3f", gmail, smtp)
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

func TestOverTargetLosesToStickinessButNotToEveryPenalty(t *testing.T) {
	over, room := uuid.New(), uuid.New()

	// Stickiness alone never rescues an over-target worker: the flat cost is
	// set above incumbency plus a region match.
	a := candidate(over, 16, 18)
	a.Region = "eu"
	b := candidate(room, 16, 15)
	got := SelectPlacement([]PlacementCandidate{a, b}, PlacementRequest{
		Weight: 1.0, CurrentWorkerID: &over, Region: "eu",
	})
	if got == nil || got.WorkerID != room {
		t.Fatal("stickiness must not outweigh being over target")
	}

	// It is not a fence, though. Stack every penalty on the worker with room -
	// a fresh node full of foreign tenants, already holding all of this org's
	// mailboxes and crowded with this provider - and a marginally over-target
	// but otherwise clean worker is the better home. That is the intended
	// reading: the other terms are real costs, not tie-breakers.
	crowded := candidate(room, 16, 15)
	crowded.Capacity.AgeMul = 0
	crowded.TotalMailboxes = 1000
	crowded.OrgMailboxesHere = 10
	crowded.ProviderMailboxesHere = 24
	marginal := candidate(over, 16, 16.05)
	req := PlacementRequest{Weight: 0.05, IsolatedEgress: true, OrgMailboxesTotal: 10}
	if marginal.Score(req) <= crowded.Score(req) {
		t.Fatalf("over-target cost should not dominate every penalty: over=%.3f room=%.3f",
			marginal.Score(req), crowded.Score(req))
	}
}

func TestNewNodeRelievesAFullFleet(t *testing.T) {
	fresh, mature := uuid.New(), uuid.New()

	// A node enrolled an hour ago keeps its target while age remains a score.
	n := ComputeCapacity(WorkerCapacityRow{BaseCapacity: 16, HealthMultiplier: 1, AgeMultiplier: 1.0 / 72, LoadScore: 1})
	newNode := PlacementCandidate{WorkerID: fresh, Health: models.WorkerHealthHealthy, Capacity: n}
	full := candidate(mature, 16, 20)

	got := SelectPlacement([]PlacementCandidate{full, newNode}, PlacementRequest{Weight: 1.0})
	if got == nil || got.WorkerID != fresh {
		t.Fatal("a freshly joined node must be able to relieve an over-target fleet")
	}
}

func TestYouthIsAPreferenceNotABarrier(t *testing.T) {
	fresh, mature := uuid.New(), uuid.New()

	young := candidate(fresh, 16, 0)
	young.Capacity.AgeMul = 0
	old := candidate(mature, 16, 0)

	got := SelectPlacement([]PlacementCandidate{young, old}, PlacementRequest{Weight: 1.0})
	if got == nil || got.WorkerID != mature {
		t.Fatal("between two empty workers the proven one should win")
	}
}
