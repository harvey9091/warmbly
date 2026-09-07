package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/models"
)

// EmailOpen is one recorded open of one step: when, from what client and
// where. The progress row keeps only the first open per step; this keeps
// them all, machine ones included and labelled, for the contact's timeline
// and the campaign's audience breakdown.
type EmailOpen struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	CampaignID    uuid.UUID
	ContactID     uuid.UUID
	SequenceID    uuid.UUID
	OpenedAt      time.Time
	Machine       bool
	MachineReason string
	UserAgent     string
	IPHash        string
	Origin        models.EngagementOrigin
}

// Machine-open reasons stored in email_opens.machine_reason.
const (
	EmailOpenReasonPrefetch = "prefetch" // a mail client prefetch or a fetch with no browser
	EmailOpenReasonInstant  = "instant"  // arrived inside the machine window after dispatch
)

// EmailOpenRepository is the per-event open log. Only the tracking consumer
// writes it.
type EmailOpenRepository interface {
	Insert(ctx context.Context, open *EmailOpen) error
	// HasHumanOpen reports whether a person's own open is on record for the
	// step, which decides whether an open a click implied can be withdrawn.
	HasHumanOpen(ctx context.Context, campaignID, contactID, sequenceID uuid.UUID) (bool, error)
	// Cleanup deletes opens older than the retention window.
	Cleanup(ctx context.Context, olderThanDays int) (int64, error)
}

type emailOpenRepository struct {
	db *pgxpool.Pool
}

// NewEmailOpenRepository creates a new email open repository.
func NewEmailOpenRepository(db *pgxpool.Pool) EmailOpenRepository {
	return &emailOpenRepository{db: db}
}

func (r *emailOpenRepository) Insert(ctx context.Context, o *EmailOpen) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	if o.OpenedAt.IsZero() {
		o.OpenedAt = time.Now()
	}
	query := `
		INSERT INTO email_opens
			(id, task_id, campaign_id, contact_id, sequence_id, opened_at,
			 machine, machine_reason, user_agent, ip_hash,
			 client, device_type, os, browser, browser_version, country_code, region, city)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`
	_, err := r.db.Exec(ctx, query,
		o.ID, o.TaskID, o.CampaignID, o.ContactID, o.SequenceID, o.OpenedAt,
		o.Machine, o.MachineReason, o.UserAgent, o.IPHash,
		o.Origin.Client, o.Origin.DeviceType, o.Origin.OS, o.Origin.Browser, o.Origin.BrowserVersion,
		o.Origin.CountryCode, o.Origin.Region, o.Origin.City,
	)
	return err
}

func (r *emailOpenRepository) HasHumanOpen(ctx context.Context, campaignID, contactID, sequenceID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM email_opens
			WHERE campaign_id = $1 AND contact_id = $2 AND sequence_id = $3 AND machine = false
		)
	`
	var ok bool
	err := r.db.QueryRow(ctx, query, campaignID, contactID, sequenceID).Scan(&ok)
	return ok, err
}

func (r *emailOpenRepository) Cleanup(ctx context.Context, olderThanDays int) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM email_opens WHERE opened_at < NOW() - $1 * INTERVAL '1 day'`,
		olderThanDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
