package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// ContactSendConstraint names the gate holding a step back; the caller words it.
type ContactSendConstraint string

const (
	ConstraintNone             ContactSendConstraint = ""
	ConstraintStepWait         ContactSendConstraint = "step_wait"
	ConstraintEntryDelay       ContactSendConstraint = "entry_delay"
	ConstraintConditionWindow  ContactSendConstraint = "condition_window"
	ConstraintStartDate        ContactSendConstraint = "start_date"
	ConstraintSendingWindow    ContactSendConstraint = "sending_window"
	ConstraintNewLeadCap       ContactSendConstraint = "new_lead_cap"
	ConstraintCapacity         ContactSendConstraint = "capacity"
	ConstraintCampaignInactive ContactSendConstraint = "campaign_inactive"
	ConstraintNoMailbox        ContactSendConstraint = "no_mailbox"
	ConstraintDomainAuth       ContactSendConstraint = "domain_auth"
	ConstraintCampaignEnded    ContactSendConstraint = "campaign_ended"
)

// ContactSendPreview is a read-only "what happens next" for one contact in
// one campaign; ScheduledAt is set only when the step is due now.
type ContactSendPreview struct {
	Route       *repository.ContactRoute
	State       models.ContactNextActionState
	ScheduledAt *time.Time
	NotBefore   *time.Time
	Constraint  ContactSendConstraint
}

// ContactSendPreviewer is satisfied by the scheduler service.
type ContactSendPreviewer interface {
	PreviewContactSend(ctx context.Context, campaignID, contactID uuid.UUID) (*ContactSendPreview, error)
}

// PreviewContactSend routes and places one contact through the send path's
// own rules without writing anything.
func (s *schedulerService) PreviewContactSend(ctx context.Context, campaignID, contactID uuid.UUID) (*ContactSendPreview, error) {
	campaign, err := s.campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	route, err := s.campaignProgressRepo.RouteContact(ctx, campaignID, contactID)
	if err != nil {
		return nil, err
	}
	pv := &ContactSendPreview{Route: route}

	if route.WaitUntil != nil {
		pv.State = models.NextActionWaiting
		pv.NotBefore = route.WaitUntil
		pv.Constraint = ConstraintConditionWindow
		return pv, nil
	}
	if route.Target == nil || route.Excluded != "" {
		return pv, nil
	}
	if campaign.Status != "active" {
		pv.State = models.NextActionPaused
		pv.NotBefore = route.DueAt
		pv.Constraint = ConstraintCampaignInactive
		return pv, nil
	}

	projectRampLevel(campaign, time.Now())

	accounts, meta, err := s.campaignSenders(ctx, campaign)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		pv.State = models.NextActionBlocked
		pv.Constraint = ConstraintNoMailbox
		return pv, nil
	}

	pair := &repository.ContactSequencePair{ContactID: contactID, SequenceID: *route.Target, IsNewLead: route.IsNewLead, NotBefore: route.DueAt}
	at, sendable, _, perr := s.placeCampaignSend(ctx, campaign, accounts, meta, pair, true)
	switch {
	case perr == nil && sendable != nil:
		pv.State = models.NextActionDue
		slot := at
		pv.ScheduledAt = &slot
		return pv, nil
	case errors.Is(perr, ErrCampaignDeferred):
		pv.State = models.NextActionWaiting
		slot := at
		if route.DueAt != nil && route.DueAt.After(slot) {
			slot = *route.DueAt
		}
		pv.NotBefore = &slot
		pv.Constraint = s.deferralConstraint(ctx, campaign, route)
		return pv, nil
	case errors.Is(perr, ErrCampaignEnded):
		pv.State = models.NextActionBlocked
		pv.Constraint = ConstraintCampaignEnded
		return pv, nil
	case errors.Is(perr, ErrDomainAuthFailing):
		pv.State = models.NextActionBlocked
		pv.Constraint = ConstraintDomainAuth
		return pv, nil
	case errors.Is(perr, ErrNoEmailAccounts):
		pv.State = models.NextActionBlocked
		pv.Constraint = ConstraintCapacity
		return pv, nil
	}
	return nil, perr
}

// deferralConstraint picks the gate that explains a deferred placement, in
// the order the send path applies them: the step's own wait (or, for a first
// step, the campaign's entry delay), the campaign start date, the sending
// window, today's new-lead cap, and otherwise the mailbox pool (daily caps,
// spacing, health, rest).
func (s *schedulerService) deferralConstraint(ctx context.Context, campaign *models.Campaign, route *repository.ContactRoute) ContactSendConstraint {
	grace := time.Now().Add(config.CampaignNotDueGraceSeconds * time.Second)
	if route.DueAt != nil && route.DueAt.After(grace) {
		// A first step's wait is the campaign's entry delay, not a step wait —
		// the drawer words the two differently.
		if route.IsNewLead {
			return ConstraintEntryDelay
		}
		return ConstraintStepWait
	}
	if campaign.StartDate != nil && campaign.StartDate.After(grace) {
		return ConstraintStartDate
	}
	if nextScheduleSlot(time.Now(), effectiveWindows(campaign), loadLocation(campaign.Timezone)).After(grace) {
		return ConstraintSendingWindow
	}
	if route.IsNewLead && campaign.MaxNewLeadsPerDay > 0 {
		if n, err := s.campaignRepo.CountNewLeadsStartedToday(ctx, campaign.ID); err == nil && n >= campaign.MaxNewLeadsPerDay {
			return ConstraintNewLeadCap
		}
	}
	return ConstraintCapacity
}

// projectRampLevel applies today's ramp advance in memory, mirroring
// campaignRepo.AdvanceRampLevel, so a preview on a new UTC day budgets with
// the level the next real pass will persist instead of yesterday's.
func projectRampLevel(c *models.Campaign, now time.Time) {
	if !c.RampEnabled {
		return
	}
	today := now.UTC().Truncate(24 * time.Hour)
	if c.RampLevelDate != nil && !c.RampLevelDate.UTC().Truncate(24*time.Hour).Before(today) {
		return
	}
	c.RampLevel = min(c.RampCeiling, max(c.RampLevel, c.RampStart)+c.RampIncrement)
	c.RampLevelDate = &today
}
