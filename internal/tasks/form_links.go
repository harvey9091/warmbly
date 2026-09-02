package tasks

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
)

// FormLinkMinter is the slice of form.Service the send pipeline needs to
// turn a {{form_link:<public_id>}} marker into a per-recipient URL. It's
// satisfied structurally so tasks needs no import of the form package
// (mirrors the AutomationRunner wiring pattern).
type FormLinkMinter interface {
	MintForContact(ctx context.Context, orgID uuid.UUID, formPublicID string, contactID uuid.UUID, campaignID *uuid.UUID) (personalURL, shareURL string, ok bool)
}

// resolveFormLinks replaces every {{form_link:<public_id>}} marker with that
// recipient's personalized form URL. It runs BEFORE RenderTemplate, so the
// substituted literal URL survives the naive fallback path too and is then
// wrapped by click tracking like any other link. Resolution never fails a
// send: an unknown, unpublished or foreign form logs a warning and the
// marker is dropped rather than shipping broken syntax to a recipient.
func (s *tasksService) resolveFormLinks(ctx context.Context, orgID uuid.UUID, campaign *models.Campaign, contact *models.Contact, parts ...*string) {
	for _, part := range parts {
		if part == nil || *part == "" {
			continue
		}
		*part = models.FormLinkMarkerRE.ReplaceAllStringFunc(*part, func(marker string) string {
			publicID := models.FormLinkMarkerRE.FindStringSubmatch(marker)[1]
			if s.formLinks == nil {
				log.Warn().Str("form", publicID).Msg("form_link marker with no minter wired")
				return ""
			}
			personal, _, ok := s.formLinks.MintForContact(ctx, orgID, publicID, contact.ID, &campaign.ID)
			if !ok {
				log.Warn().Str("form", publicID).Str("campaign_id", campaign.ID.String()).Msg("form_link marker did not resolve")
				return ""
			}
			return personal
		})
	}
}
