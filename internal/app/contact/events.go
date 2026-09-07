package contact

import (
	"context"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

type skipCreatedEventsKey struct{}

// contactCreatedEventMaxBatch is the largest single write that still raises
// contact.created per row. A request adding more contacts at once is a bulk
// arrival like a file import, and stays silent for the same reason.
const contactCreatedEventMaxBatch = 100

// WithoutCreatedEvents marks a context whose contact writes must not fire
// contact.created. Bulk arrivals (file import, sheet sync) use it so one
// upload cannot flood an organization's automations and webhooks.
func WithoutCreatedEvents(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipCreatedEventsKey{}, true)
}

func createdEventsSuppressed(ctx context.Context) bool {
	v, _ := ctx.Value(skipCreatedEventsKey{}).(bool)
	return v
}

// emitCreated fires contact.created for every row the upsert inserted. Rows
// that matched an existing contact stay silent: their first touch already
// happened, and so does a batch past contactCreatedEventMaxBatch. in and out
// are index-aligned, as the repository returns them.
func (s *contactService) emitCreated(ctx context.Context, orgID uuid.UUID, in []models.AddContact, out []models.Contact) {
	if s.webhooks == nil || createdEventsSuppressed(ctx) || len(in) > contactCreatedEventMaxBatch {
		return
	}
	for i := range out {
		if !out[i].IsNew {
			continue
		}
		var src models.AddContact
		if i < len(in) {
			src = in[i]
		}
		_, _ = s.webhooks.Dispatch(ctx, orgID, models.WebhookEventContactCreated, ContactCreatedPayload(out[i], src))
	}
}

// ContactCreatedPayload is the contact.created event body: the contact's
// fields plus its first-touch source, flat so automation conditions and
// templates read them as {{.contact_email}}, {{.first_name}}, {{.source}}.
func ContactCreatedPayload(c models.Contact, src models.AddContact) map[string]any {
	source := src.Source
	if source == "" {
		source = models.ContactSourceUnknown
	}
	custom := c.CustomFields
	if custom == nil {
		custom = map[string]string{}
	}
	campaignIDs := make([]string, 0, len(c.Campaigns))
	for _, cm := range c.Campaigns {
		campaignIDs = append(campaignIDs, cm.ID)
	}
	categoryIDs := make([]string, 0, len(c.Categories))
	for _, cat := range c.Categories {
		categoryIDs = append(categoryIDs, cat.ID.String())
	}
	return map[string]any{
		"contact_id":    c.ID.String(),
		"contact_email": c.Email,
		"first_name":    c.FirstName,
		"last_name":     c.LastName,
		"company":       c.Company,
		"phone":         c.Phone,
		"subscribed":    c.Subscribed,
		"custom_fields": custom,
		"source":        string(source),
		"source_detail": src.SourceDetail,
		"campaign_ids":  campaignIDs,
		"category_ids":  categoryIDs,
		"created_at":    c.CreatedAt,
	}
}
