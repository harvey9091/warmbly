package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// FormLinkRepository stores per-contact form URL tickets. The row id is the
// opaque token that travels in ?t=; there is no signing secret anywhere.
type FormLinkRepository interface {
	// Upsert returns the stable token for (form, contact). The insert is
	// guarded against the contact's org so a foreign contact id cannot be
	// linked; campaign_id keeps the first non-null value.
	Upsert(ctx context.Context, orgID, formID, contactID uuid.UUID, campaignID *uuid.UUID) (uuid.UUID, *errx.Error)
	// Resolve is the public path: token to link, no org filter (the uuid is
	// the capability, like tracked_links).
	Resolve(ctx context.Context, token uuid.UUID) (*models.FormLink, *errx.Error)
}

type formLinkRepository struct {
	DB *db.DB
}

func NewFormLinkRepository(d *db.DB) FormLinkRepository {
	return &formLinkRepository{DB: d}
}

func (r *formLinkRepository) Upsert(ctx context.Context, orgID, formID, contactID uuid.UUID, campaignID *uuid.UUID) (uuid.UUID, *errx.Error) {
	var id uuid.UUID
	err := r.DB.QueryRow(ctx, `
		INSERT INTO form_links (form_id, organization_id, contact_id, campaign_id)
		SELECT $2, $1, c.id, $4 FROM contacts c WHERE c.id = $3 AND c.organization_id = $1
		ON CONFLICT (form_id, contact_id)
			DO UPDATE SET campaign_id = COALESCE(form_links.campaign_id, EXCLUDED.campaign_id)
		RETURNING id
	`, orgID, formID, contactID, campaignID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errx.New(errx.NotFound, "contact not found")
	}
	if err != nil {
		db.CaptureError(err, "form links upsert", nil, "query")
		return uuid.Nil, errx.InternalError()
	}
	return id, nil
}

func (r *formLinkRepository) Resolve(ctx context.Context, token uuid.UUID) (*models.FormLink, *errx.Error) {
	var l models.FormLink
	err := r.DB.QueryRow(ctx, `
		SELECT id, form_id, organization_id, contact_id, campaign_id, created_at
		FROM form_links WHERE id = $1
	`, token).Scan(&l.ID, &l.FormID, &l.OrganizationID, &l.ContactID, &l.CampaignID, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errx.New(errx.NotFound, "link not found")
	}
	if err != nil {
		db.CaptureError(err, "form links resolve", nil, "query")
		return nil, errx.InternalError()
	}
	return &l, nil
}
