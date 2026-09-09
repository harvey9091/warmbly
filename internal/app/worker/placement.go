// Placement is the scoring layer that replaced the old category filters.
//
// Workers used to be partitioned four ways (free_tier, worker_type, risk_pool,
// egress_kind) and placement was a filter: find a worker whose labels match
// the mailbox's labels. That model assumed the worker's IP was the sending
// identity, the way it is for a platform that talks to recipient MXs directly.
// Warmbly doesn't. A worker authenticates to the customer's own mailbox
// provider and that provider delivers from its own outbound pool, so:
//
//   - the worker IP is invisible to recipient spam filtering (Google strips
//     the submitting client's IP; Microsoft dropped X-Originating-IP), which
//     means co-locating a spam-prone mailbox next to a healthy one cannot
//     contaminate the healthy one's sending reputation
//   - the worker IP is very visible to the mailbox provider, where it drives
//     sign-in risk challenges, per-IP auth throttles (454 4.7.0) and per-IP
//     rate limits (421 4.7.28)
//
// So the levers invert. IP *stability* per mailbox beats IP diversity, because
// moving a mailbox changes the client IP its provider sees and buys a security
// challenge for nothing. Placement stops being a partition problem and becomes
// a scoring problem: spend capacity well, keep mailboxes still, keep one
// client IP from crowding one provider, and keep any single worker from
// carrying too much of one customer.

package worker

import (
	"sort"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// Placement scoring weights. Relative magnitudes are the design: stickiness
// outranks packing, because a needless migration costs provider trust while a
// slightly hotter worker costs nothing a rebalance can't fix later.
const (
	weightHeadroom     = 1.0
	weightIncumbent    = 2.0
	weightRegion       = 0.6
	weightIsolation    = 1.5
	weightBlastRadius  = 0.8
	weightProviderLoad = 0.7
)

// providerSoftCap is how many mailboxes of ONE provider a single worker is
// expected to carry before the placer starts steering elsewhere. It is a soft
// cap: exceeding it lowers the score, it never refuses a placement. Anchored
// on the provider-side limits rather than anything about our machines -
// Exchange Online allows ~3 concurrent SMTP AUTH connections and ~30 msg/min
// per mailbox, and Gmail rate-limits authentication per client IP, so the
// thing worth spreading is how many accounts of one provider sign in from one
// address.
const providerSoftCap = 24.0

// PlacementCandidate is one worker the placer may choose, with the live facts
// the score reads. Everything here is observed, none of it is configured.
type PlacementCandidate struct {
	WorkerID uuid.UUID
	Region   string
	Capacity Capacity
	Health   models.WorkerHealthState

	// TotalMailboxes is every mailbox currently on this worker.
	TotalMailboxes int
	// OrgMailboxesHere is how many of them belong to the org being placed.
	OrgMailboxesHere int
	// ProviderMailboxesHere is how many of them use the same mailbox provider
	// as the one being placed, from any org. Same client IP, same provider,
	// same rate-limit bucket.
	ProviderMailboxesHere int
}

// PlacementRequest describes the mailbox that needs a home.
type PlacementRequest struct {
	// Weight is the mailbox's load contribution (see MailboxWeight).
	Weight float64
	// Region the mailbox's provider expects sign-ins from. Empty means no
	// preference and scores neutral rather than penalising anything.
	Region string
	// CurrentWorkerID is the incumbent, when this is a re-placement. The
	// incumbent gets a large bonus: staying put is the default.
	CurrentWorkerID *uuid.UUID
	// OrgMailboxesTotal is how many mailboxes the org owns in total, the
	// denominator for the blast-radius term. Zero disables that term.
	OrgMailboxesTotal int
	// IsolatedEgress is the org entitlement that buys neighbours-free egress.
	// It does not pin the org to a machine; it makes foreign tenants expensive
	// to score against, so the fleet converges on isolation and self-heals
	// when a worker dies instead of stranding the customer.
	IsolatedEgress bool
}

// Eligible reports whether a candidate may host the mailbox at all. These are
// the only hard constraints left: the worker has to be able to do the work.
// Everything else is a preference expressed in the score.
func (c PlacementCandidate) Eligible(req PlacementRequest) bool {
	switch c.Health {
	case models.WorkerHealthHealthy, models.WorkerHealthWatch:
	default:
		return false
	}
	return c.Capacity.Effective-c.Capacity.Load >= req.Weight
}

// Score ranks an eligible candidate. Higher is better. The terms are additive
// and each one is traceable to a specific provider behaviour, which is what
// makes the number explainable in the decision log.
func (c PlacementCandidate) Score(req PlacementRequest) float64 {
	var score float64

	// Capacity: prefer the worker with the most room, so the fleet fills evenly.
	utilization := c.Capacity.Utilization
	if utilization > 1 {
		utilization = 1
	}
	score += weightHeadroom * (1 - utilization)

	// Stickiness: the incumbent wins ties and most non-ties. A mailbox that
	// stays put keeps presenting the same client IP to its provider.
	if req.CurrentWorkerID != nil && *req.CurrentWorkerID == c.WorkerID {
		score += weightIncumbent
	}

	// Sign-in geography: a mailbox whose provider sees logins from the region
	// it expects raises fewer risk challenges. Unknown on either side is
	// neutral, never a penalty - most installs set no region at all.
	if req.Region != "" && c.Region != "" && req.Region == c.Region {
		score += weightRegion
	}

	// Neighbours: only orgs entitled to isolated egress pay attention to who
	// else is on the box, and it is a preference, not a refusal.
	if req.IsolatedEgress && c.TotalMailboxes > 0 {
		foreign := float64(c.TotalMailboxes-c.OrgMailboxesHere) / float64(c.TotalMailboxes)
		score -= weightIsolation * foreign
	}

	// Blast radius: if this worker dies or its IP gets throttled, how much of
	// this customer's sending stops? Spread the org across workers.
	if req.OrgMailboxesTotal > 0 {
		concentration := float64(c.OrgMailboxesHere) / float64(req.OrgMailboxesTotal)
		if concentration > 1 {
			concentration = 1
		}
		score -= weightBlastRadius * concentration
	}

	// Provider crowding: many accounts of one provider authenticating from one
	// address is what earns a per-IP auth throttle. Soft, and it saturates.
	crowding := float64(c.ProviderMailboxesHere) / providerSoftCap
	if crowding > 1 {
		crowding = 1
	}
	score -= weightProviderLoad * crowding

	return score
}

// SelectPlacement picks the best eligible candidate, or nil when none can take
// the mailbox. Deterministic: ties break on worker id so two concurrent
// placements of identical mailboxes agree, and a test can assert an outcome.
func SelectPlacement(candidates []PlacementCandidate, req PlacementRequest) *PlacementCandidate {
	type scored struct {
		cand  PlacementCandidate
		value float64
	}
	eligible := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		if !c.Eligible(req) {
			continue
		}
		eligible = append(eligible, scored{cand: c, value: c.Score(req)})
	}
	if len(eligible) == 0 {
		return nil
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].value != eligible[j].value {
			return eligible[i].value > eligible[j].value
		}
		return eligible[i].cand.WorkerID.String() < eligible[j].cand.WorkerID.String()
	})
	best := eligible[0].cand
	return &best
}
