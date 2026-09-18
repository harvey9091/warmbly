package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

// recentPartnerRepo reports partners used in the wider history window.
type recentPartnerRepo struct {
	candidateRepo

	recent []uuid.UUID
}

func (r recentPartnerRepo) GetRecentlyUsedPartners(_ context.Context, _ uuid.UUID, since time.Time) ([]uuid.UUID, error) {
	// Keep the partner eligible today while marking it recently used.
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

// Outside partners must beat a candidate set dominated by siblings.
func TestSelectWarmupPartnerPrefersAPartnerOutsideTheSendersWorkspace(t *testing.T) {
	gate := &rejectingGate{poolOf: map[uuid.UUID]string{}}
	outside := foreignPartner(gate)
	cands := []models.WarmupPartnerCandidate{outside}
	// Repeated draws make loss of the preference deterministic enough to catch.
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

// Workspace diversity outranks freshness.
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

// Siblings remain eligible for a single-workspace self-hosted pool.
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

// Unknown ownership cannot prove that a candidate is a sibling.
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
