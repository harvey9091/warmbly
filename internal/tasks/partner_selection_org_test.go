package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

// recentPartnerRepo reports the listed partners as recently used, which is
// what demotes them within their workspace tier.
type recentPartnerRepo struct {
	candidateRepo

	recent []uuid.UUID
}

func (r recentPartnerRepo) GetRecentlyUsedPartners(_ context.Context, _ uuid.UUID, since time.Time) ([]uuid.UUID, error) {
	// The selector asks twice: once over the 72h window and once for today.
	// Only the wider question is answered, so the partner is "recently used"
	// without being "used today", which would skip it altogether.
	if time.Since(since) < 24*time.Hour {
		return nil, nil
	}
	return r.recent, nil
}

// sibling is a candidate owned by the same workspace as the sender.
func sibling(org uuid.UUID, gate *rejectingGate) models.WarmupPartnerCandidate {
	c := models.WarmupPartnerCandidate{ID: uuid.New(), Email: "sibling@own.test", OrganizationID: &org}
	gate.poolOf[c.ID] = "premium"
	return c
}

// foreignPartner is a candidate owned by somebody else.
func foreignPartner(gate *rejectingGate) models.WarmupPartnerCandidate {
	other := uuid.New()
	c := models.WarmupPartnerCandidate{ID: uuid.New(), Email: "partner@elsewhere.test", OrganizationID: &other}
	gate.poolOf[c.ID] = "premium"
	return c
}

// A customer who brings twenty mailboxes used to become most of its own
// candidate set, so most of its warmup mail went to its own domains: traffic
// that teaches the providers it will cold-mail nothing (#575).
func TestSelectWarmupPartnerPrefersAPartnerOutsideTheSendersWorkspace(t *testing.T) {
	gate := &rejectingGate{poolOf: map[uuid.UUID]string{}}
	outside := foreignPartner(gate)
	cands := []models.WarmupPartnerCandidate{outside}
	// Nineteen siblings against one outsider: losing the preference is a
	// near-certain failure over twenty draws, not a coin flip.
	s, sender := premiumSelector(gate)
	for i := 0; i < 19; i++ {
		cands = append(cands, sibling(*sender.OrganizationID, gate))
	}
	s.warmupRepo = candidateRepo{candidates: cands}

	for i := 0; i < 20; i++ {
		partner, err := s.selectWarmupPartner(context.Background(), sender)
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if partner.ID != outside.ID {
			t.Fatalf("draw %d paired the sender with its own mailbox %s", i, partner.ID)
		}
	}
}

// A recently used outside partner still beats a fresh sibling: freshness is a
// tie-break within a workspace tier, not a reason to close the loop.
func TestSelectWarmupPartnerPrefersARecentOutsiderOverAFreshSibling(t *testing.T) {
	gate := &rejectingGate{poolOf: map[uuid.UUID]string{}}
	outside := foreignPartner(gate)
	s, sender := premiumSelector(gate)
	own := sibling(*sender.OrganizationID, gate)
	s.warmupRepo = recentPartnerRepo{
		candidateRepo: candidateRepo{candidates: []models.WarmupPartnerCandidate{outside, own}},
		recent:        []uuid.UUID{outside.ID},
	}

	partner, err := s.selectWarmupPartner(context.Background(), sender)
	if err != nil {
		t.Fatal(err)
	}
	if partner.ID != outside.ID {
		t.Fatalf("drew the sibling %s over the recently used outsider %s", partner.ID, outside.ID)
	}
}

// The preference is not an exclusion: on a self-hosted instance the pool IS
// one workspace, so refusing a sibling would leave warmup with no partner at
// all and stop it dead.
func TestSelectWarmupPartnerStillWarmsWhenEveryPartnerIsASibling(t *testing.T) {
	gate := &rejectingGate{poolOf: map[uuid.UUID]string{}}
	s, sender := premiumSelector(gate)
	own := sibling(*sender.OrganizationID, gate)
	s.warmupRepo = candidateRepo{candidates: []models.WarmupPartnerCandidate{own}}

	partner, err := s.selectWarmupPartner(context.Background(), sender)
	if err != nil {
		t.Fatalf("a single-workspace pool warmed nobody: %v", err)
	}
	if partner.ID != own.ID {
		t.Fatalf("drew %s, want the only candidate %s", partner.ID, own.ID)
	}
}

// An unattributable mailbox is never read as the sender's own. The column has
// been NOT NULL since migration 000092, so this guards the Go-level pointer
// rather than a row: a nil owner cannot be shown to be the same workspace, and
// guessing would push a legitimate partner behind the sender's own siblings.
func TestSelectWarmupPartnerTreatsAnOwnerlessMailboxAsOutside(t *testing.T) {
	gate := &rejectingGate{poolOf: map[uuid.UUID]string{}}
	orphan := models.WarmupPartnerCandidate{ID: uuid.New(), Email: "orphan@nowhere.test"}
	s, sender := premiumSelector(gate)
	gate.poolOf[orphan.ID] = "premium"
	own := sibling(*sender.OrganizationID, gate)
	s.warmupRepo = candidateRepo{candidates: []models.WarmupPartnerCandidate{own, orphan}}

	for i := 0; i < 20; i++ {
		partner, err := s.selectWarmupPartner(context.Background(), sender)
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if partner.ID != orphan.ID {
			t.Fatalf("draw %d drew the sibling %s over the unowned candidate %s", i, partner.ID, orphan.ID)
		}
	}
}
