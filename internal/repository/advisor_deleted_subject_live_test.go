package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedAdvisorFinding(t *testing.T, pool *pgxpool.Pool, org uuid.UUID, key, entityType string, entityID uuid.UUID, parentID *uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	parentType := ""
	if parentID != nil {
		parentType = "campaign"
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO advisor_findings (organization_id, fingerprint, detector_key, category, severity, surface,
		    entity_type, entity_id, parent_type, parent_id, title)
		VALUES ($1, $2, $3, 'mailbox', 'high', 'mailboxes', $4, $5, $6, $7, 'Live finding')
		RETURNING id`,
		org, key+":"+entityType+":"+entityID.String(), key, entityType, entityID, parentType, parentID,
	).Scan(&id); err != nil {
		t.Fatalf("seed finding: %v", err)
	}
	return id
}

func findingStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM advisor_findings WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read finding: %v", err)
	}
	return status
}

// Advice about a mailbox, campaign or step leaves with it, in the delete's own
// transaction, rather than lingering on the page until the next evaluation.
func TestLiveAdvisorFindingsCloseWithTheirSubject(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newLifecycleFixture(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM advisor_findings WHERE organization_id = $1`, f.org); err != nil {
			t.Errorf("cleanup findings: %v", err)
		}
	})

	mailboxFinding := seedAdvisorFinding(t, pool, f.org, "mailbox_cap_too_high", "email_account", f.mailbox, nil)
	stepFinding := seedAdvisorFinding(t, pool, f.org, "step_spammy", "step", f.steps[1], &f.campaign)
	siblingFinding := seedAdvisorFinding(t, pool, f.org, "step_spammy", "step", f.steps[0], &f.campaign)
	campaignFinding := seedAdvisorFinding(t, pool, f.org, "campaign_no_unsub", "campaign", f.campaign, nil)
	orgFinding := seedAdvisorFinding(t, pool, f.org, "mailbox_concentration", "", uuid.New(), nil)

	if xerr := NewSequenceRepostory(handle).Delete(ctx, f.org.String(), f.campaign.String(), f.steps[1].String()); xerr != nil {
		t.Fatalf("delete step: %v", xerr)
	}
	if got := findingStatus(t, pool, stepFinding); got != "resolved" {
		t.Fatalf("deleted step's finding = %q, want resolved", got)
	}
	if got := findingStatus(t, pool, siblingFinding); got != "open" {
		t.Fatalf("surviving step's finding = %q, want open", got)
	}

	if err := NewCampaignRepostory(handle).Delete(ctx, f.campaign); err != nil {
		t.Fatalf("delete campaign: %v", err)
	}
	for name, id := range map[string]uuid.UUID{"campaign": campaignFinding, "its remaining step": siblingFinding} {
		if got := findingStatus(t, pool, id); got != "resolved" {
			t.Fatalf("%s finding = %q after the campaign went, want resolved", name, got)
		}
	}

	if xerr := NewEmailRepostory(handle, nil).Delete(ctx, f.org.String(), f.mailbox.String(), 1); xerr != nil {
		t.Fatalf("delete mailbox: %v", xerr)
	}
	if got := findingStatus(t, pool, mailboxFinding); got != "resolved" {
		t.Fatalf("deleted mailbox's finding = %q, want resolved", got)
	}

	// The org-wide finding is the evaluator's to close, and the org must still
	// be due for one now that it has no mailbox left.
	if got := findingStatus(t, pool, orgFinding); got != "open" {
		t.Fatalf("org finding = %q, want open until an evaluation closes it", got)
	}
	due, err := NewAdvisorRepository(handle).ListOrgsDue(ctx, time.Hour, 100000)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	found := false
	for _, id := range due {
		found = found || id == f.org
	}
	if !found {
		t.Fatal("an org with open advice and no mailbox is never re-evaluated, so its advice never closes")
	}
}
