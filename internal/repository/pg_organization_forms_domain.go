package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OrgFormsDomain is one organization's custom forms domain, for the sweep.
type OrgFormsDomain struct {
	OrganizationID uuid.UUID
	Domain         string
	Verified       bool
}

func (r *organizationRepository) GetFormsDomain(ctx context.Context, orgID uuid.UUID) (string, bool, error) {
	var domain string
	var verified bool
	err := r.db.QueryRow(ctx, `
		SELECT forms_domain, forms_domain_verified FROM organizations WHERE id = $1
	`, orgID).Scan(&domain, &verified)
	if err != nil {
		return "", false, err
	}
	return domain, verified, nil
}

// SetFormsDomain stores a new value and clears the verdict: the old one
// describes a name that is no longer configured.
func (r *organizationRepository) SetFormsDomain(ctx context.Context, orgID uuid.UUID, domain string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE organizations
		SET forms_domain = $2, forms_domain_verified = false, forms_domain_verified_at = NULL, updated_at = now()
		WHERE id = $1
	`, orgID, domain)
	return err
}

func (r *organizationRepository) SetFormsDomainVerified(ctx context.Context, orgID uuid.UUID, verified bool, at *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE organizations
		SET forms_domain_verified = $2, forms_domain_verified_at = $3, updated_at = now()
		WHERE id = $1
	`, orgID, verified, at)
	return err
}

func (r *organizationRepository) ListFormsDomains(ctx context.Context) ([]OrgFormsDomain, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, forms_domain, forms_domain_verified
		FROM organizations
		WHERE forms_domain <> '' AND deletion_scheduled_for IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []OrgFormsDomain{}
	for rows.Next() {
		var d OrgFormsDomain
		if err := rows.Scan(&d.OrganizationID, &d.Domain, &d.Verified); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
