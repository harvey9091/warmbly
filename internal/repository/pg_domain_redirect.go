package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// ErrRedirectTaken is returned when another workspace already serves a verified redirect for the domain.
var ErrRedirectTaken = errors.New("domain redirect already verified elsewhere")

// DomainRedirectRepository stores sending-domain redirects. Reads a caller can
// reach are scoped by organization; Lookup serves the tracking service by host.
type DomainRedirectRepository interface {
	Upsert(ctx context.Context, r *models.DomainRedirect, createdBy uuid.UUID) error
	Get(ctx context.Context, orgID uuid.UUID, domain string) (*models.DomainRedirect, error)
	List(ctx context.Context, orgID uuid.UUID) ([]models.DomainRedirect, error)
	Delete(ctx context.Context, orgID uuid.UUID, domain string) (bool, error)
	// SetCheck records a verification; ErrRedirectTaken when another workspace holds the verified domain.
	SetCheck(ctx context.Context, id uuid.UUID, verified bool, lastError string) error
	// Due are rows whose last check is older than their state allows.
	Due(ctx context.Context, limit int) ([]models.DomainRedirect, error)
	// Lookup is the target for a verified host: the domain itself, or its www when included.
	Lookup(ctx context.Context, host string) (string, bool, error)
}

type domainRedirectRepository struct{ DB *db.DB }

func NewDomainRedirectRepository(database *db.DB) DomainRedirectRepository {
	return &domainRedirectRepository{DB: database}
}

const redirectColumns = `id, organization_id, domain, target_url, include_www, verify_token, verified, verified_at,
	last_checked_at, last_error, created_at, created_by`

func scanRedirect(row pgx.Row, r *models.DomainRedirect) error {
	return row.Scan(&r.ID, &r.OrganizationID, &r.Domain, &r.TargetURL, &r.IncludeWWW, &r.VerifyToken, &r.Verified, &r.VerifiedAt,
		&r.LastCheckedAt, &r.LastError, &r.CreatedAt, &r.CreatedBy)
}

func (r *domainRedirectRepository) Upsert(ctx context.Context, d *models.DomainRedirect, createdBy uuid.UUID) error {
	query := `
		INSERT INTO domain_redirects (id, organization_id, domain, target_url, include_www, verify_token, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (organization_id, domain) DO UPDATE
		SET target_url = EXCLUDED.target_url, include_www = EXCLUDED.include_www, updated_at = now()
		RETURNING ` + redirectColumns
	if err := scanRedirect(r.DB.QueryRow(ctx, query, d.ID, d.OrganizationID, strings.ToLower(d.Domain), d.TargetURL, d.IncludeWWW, d.VerifyToken, createdBy), d); err != nil {
		db.CaptureError(err, query, nil, "queryrow")
		return err
	}
	return nil
}

func (r *domainRedirectRepository) Get(ctx context.Context, orgID uuid.UUID, domain string) (*models.DomainRedirect, error) {
	query := `SELECT ` + redirectColumns + ` FROM domain_redirects WHERE organization_id = $1 AND domain = lower($2)`
	var d models.DomainRedirect
	if err := scanRedirect(r.DB.QueryRow(ctx, query, orgID, domain), &d); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		db.CaptureError(err, query, nil, "queryrow")
		return nil, err
	}
	return &d, nil
}

func (r *domainRedirectRepository) list(ctx context.Context, query string, args ...any) ([]models.DomainRedirect, error) {
	rows, err := r.DB.Query(ctx, query, args...)
	if err != nil {
		db.CaptureError(err, query, nil, "query")
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DomainRedirect, 0)
	for rows.Next() {
		var d models.DomainRedirect
		if err := scanRedirect(rows, &d); err != nil {
			db.CaptureError(err, query, nil, "scan")
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *domainRedirectRepository) List(ctx context.Context, orgID uuid.UUID) ([]models.DomainRedirect, error) {
	return r.list(ctx, `SELECT `+redirectColumns+` FROM domain_redirects WHERE organization_id = $1 ORDER BY domain`, orgID)
}

func (r *domainRedirectRepository) Due(ctx context.Context, limit int) ([]models.DomainRedirect, error) {
	return r.list(ctx, `SELECT `+redirectColumns+` FROM domain_redirects
		WHERE last_checked_at IS NULL
		   OR (NOT verified AND last_checked_at < now() - interval '10 minutes' AND created_at > now() - interval '14 days')
		   OR (NOT verified AND last_checked_at < now() - interval '6 hours')
		   OR (verified AND last_checked_at < now() - interval '6 hours')
		ORDER BY last_checked_at NULLS FIRST
		LIMIT $1`, limit)
}

func (r *domainRedirectRepository) Delete(ctx context.Context, orgID uuid.UUID, domain string) (bool, error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM domain_redirects WHERE organization_id = $1 AND domain = lower($2)`, orgID, domain)
	if err != nil {
		db.CaptureError(err, "delete domain_redirects", nil, "exec")
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *domainRedirectRepository) SetCheck(ctx context.Context, id uuid.UUID, verified bool, lastError string) error {
	query := `
		UPDATE domain_redirects
		SET verified = $2, last_error = $3, last_checked_at = now(), updated_at = now(),
		    verified_at = CASE WHEN $2 AND NOT verified THEN now() WHEN NOT $2 THEN NULL ELSE verified_at END
		WHERE id = $1`
	if _, err := r.DB.Exec(ctx, query, id, verified, lastError); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_, _ = r.DB.Exec(ctx, `UPDATE domain_redirects SET last_checked_at = now(), last_error = $2 WHERE id = $1`, id,
				"Another workspace on this instance already redirects this domain.")
			return ErrRedirectTaken
		}
		db.CaptureError(err, query, nil, "exec")
		return err
	}
	return nil
}

func (r *domainRedirectRepository) Lookup(ctx context.Context, host string) (string, bool, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	query := `
		SELECT target_url FROM domain_redirects
		WHERE verified AND (domain = $1 OR (include_www AND 'www.' || domain = $1))
		LIMIT 1`
	var target string
	if err := r.DB.QueryRow(ctx, query, host).Scan(&target); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		db.CaptureError(err, query, nil, "queryrow")
		return "", false, err
	}
	return target, true, nil
}
