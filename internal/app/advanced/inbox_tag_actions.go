package advanced

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/app/inboxtag"
	"github.com/warmbly/warmbly/internal/models"
)

// InboxTagAction is one classified reply and what the workspace lets it do.
type InboxTagAction struct {
	OrganizationID uuid.UUID
	// OwnerUserID is the mailbox owner, who gets the task.
	OwnerUserID uuid.UUID
	// Sender is the address that replied; the contact and the suppression
	// entry both resolve from it.
	Sender    string
	Subject   string
	MessageID string
	Plan      inboxtag.Plan
}

// ApplyInboxTagActions executes a plan on the primitives a member's own click
// uses (lead hold, CRM task, suppression entry) and returns what landed. Each
// action is best-effort and independent.
func (s *service) ApplyInboxTagActions(ctx context.Context, in InboxTagAction) []string {
	sender := strings.ToLower(strings.TrimSpace(in.Sender))
	if in.OrganizationID == uuid.Nil || sender == "" || in.Plan.Empty() {
		return nil
	}
	var done []string

	var contactID *uuid.UUID
	if s.contactRepo != nil {
		if contact, xerr := s.contactRepo.GetByEmailAndOrganization(ctx, in.OrganizationID, sender); xerr == nil && contact != nil {
			contactID = &contact.ID
		}
	}

	if in.Plan.Suppress != "" {
		if s.suppressFromReply(ctx, in, sender) {
			done = append(done, inboxtag.ActionSuppress)
		}
		// A suppressed address has no sequence left to hold or chase.
		return done
	}

	if contactID != nil && s.campaignProgressRepo != nil {
		if in.Plan.HoldDays > 0 {
			until := time.Now().UTC().AddDate(0, 0, in.Plan.HoldDays)
			held, err := s.campaignProgressRepo.HoldLeadEverywhere(ctx, *contactID, &until,
				"replied not now", models.LeadHoldSourceInboxTagging)
			if err != nil {
				log.Warn().Err(err).Str("contact_id", contactID.String()).Msg("inbox tagging: not-now hold could not be written")
			} else if len(held) > 0 {
				done = append(done, inboxtag.ActionHold)
			}
		}
		if in.Plan.Stop {
			until := time.Now().UTC().AddDate(0, 0, inboxtag.StopHoldDays)
			held, err := s.campaignProgressRepo.HoldLeadEverywhere(ctx, *contactID, &until,
				"declined in a reply", models.LeadHoldSourceInboxTagging)
			if err != nil {
				log.Warn().Err(err).Str("contact_id", contactID.String()).Msg("inbox tagging: stop could not be written")
			} else if len(held) > 0 {
				done = append(done, inboxtag.ActionStop)
			}
		}
	}

	if in.Plan.Task != "" && s.crmRepo != nil && contactID != nil && in.OwnerUserID != uuid.Nil {
		owner := in.OwnerUserID
		title := in.Plan.Task + ": " + sender
		_, err := s.crmRepo.CreateCRMTask(ctx, in.OrganizationID, owner, &models.CreateCRMTask{
			ContactID:  contactID,
			Title:      models.ClampLine(title, 255),
			Priority:   "high",
			DueDate:    ptrTime(time.Now().UTC().Add(24 * time.Hour)),
			AssignedTo: &owner,
		})
		if err != nil {
			log.Warn().Err(err).Str("contact_id", contactID.String()).Msg("inbox tagging: task could not be opened")
		} else {
			done = append(done, inboxtag.ActionTask)
		}
	}
	return done
}

// suppressFromReply puts the sender on the suppression list and clears the
// contact's subscription flag, the same two writes the keyword opt-out does,
// so the send gate and the list agree.
func (s *service) suppressFromReply(ctx context.Context, in InboxTagAction, sender string) bool {
	if err := s.repo.UpsertSuppressedRecipient(ctx, &models.SuppressedRecipient{
		OrganizationID: in.OrganizationID,
		Email:          sender,
		Kind:           models.SuppressionKindEmail,
		Reason:         in.Plan.Suppress,
		Source:         models.DeliverabilityEventUnsubscribe,
		Metadata: map[string]interface{}{
			"via":        "reply",
			"classifier": "inbox_tagging",
			"message_id": in.MessageID,
		},
	}); err != nil {
		log.Warn().Err(err).Msg("inbox tagging: suppression could not be written")
		return false
	}
	if s.contactRepo != nil {
		if err := s.contactRepo.SetSubscribedByEmail(ctx, in.OrganizationID, sender, false); err != nil {
			log.Warn().Err(err).Msg("inbox tagging: could not clear the contact's subscription flag")
		}
	}
	s.emit(ctx, in.OrganizationID, models.WebhookEventCampaignUnsubscribed, map[string]any{
		"contact_email": sender,
		"source":        "reply",
		"subject":       in.Subject,
	})
	return true
}
