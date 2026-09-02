package form

import (
	"context"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

func (s *service) MintLink(ctx context.Context, orgID, formID, contactID uuid.UUID) (string, *errx.Error) {
	if s.links == nil {
		return "", errx.InternalError()
	}
	f, xerr := s.repo.Get(ctx, orgID, formID)
	if xerr != nil {
		return "", xerr
	}
	token, xerr := s.links.Upsert(ctx, orgID, formID, contactID, nil)
	if xerr != nil {
		return "", xerr
	}
	base := config.FormURLOn(s.FormsHost(ctx, orgID), f.PublicID)
	if base == "" {
		return "", errx.New(errx.BadRequest, "FORMS_DOMAIN is not configured")
	}
	return base + "?t=" + token.String(), nil
}

func (s *service) MintForContact(ctx context.Context, orgID uuid.UUID, formPublicID string, contactID uuid.UUID, campaignID *uuid.UUID) (string, string, bool) {
	f, xerr := s.repo.GetByPublicID(ctx, formPublicID)
	if xerr != nil || f.OrganizationID != orgID || f.Status != models.FormStatusPublished {
		return "", "", false
	}
	// The org's verified custom domain, so the URL that lands in an email
	// carries the customer's own name rather than the shared forms host.
	shareURL := config.FormURLOn(s.FormsHost(ctx, orgID), f.PublicID)
	if shareURL == "" {
		return "", "", false
	}
	if s.links == nil {
		return shareURL, shareURL, true
	}
	token, terr := s.links.Upsert(ctx, orgID, f.ID, contactID, campaignID)
	if terr != nil {
		// A failed mint degrades to the shared URL rather than failing a send.
		return shareURL, shareURL, true
	}
	return shareURL + "?t=" + token.String(), shareURL, true
}

func (s *service) ResolveLink(ctx context.Context, f *models.Form, token string) (*models.FormLink, map[string]string) {
	if s.links == nil || token == "" || f == nil {
		return nil, nil
	}
	id, err := uuid.Parse(token)
	if err != nil {
		return nil, nil
	}
	link, xerr := s.links.Resolve(ctx, id)
	if xerr != nil || link.FormID != f.ID {
		return nil, nil
	}
	return link, s.prefillFor(ctx, f, link.ContactID)
}

// prefillFor maps the contact's columns onto the form's mapped fields.
func (s *service) prefillFor(ctx context.Context, f *models.Form, contactID uuid.UUID) map[string]string {
	if s.contactReader == nil {
		return nil
	}
	c, xerr := s.contactReader.GetByID(ctx, contactID)
	if xerr != nil {
		return nil
	}
	values := map[string]string{
		"first_name": c.FirstName,
		"last_name":  c.LastName,
		"email":      c.Email,
		"company":    c.Company,
		"phone":      c.Phone,
	}
	out := map[string]string{}
	for _, field := range f.Fields {
		if field.MapTo == "" {
			continue
		}
		if v := values[field.MapTo]; v != "" {
			out[field.ID] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
