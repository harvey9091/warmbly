package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// FormAggregates are the per-form event rollups the forms list attaches.
type FormAggregates struct {
	Starts     int64
	Identified int64
	// Trend is daily submit counts, oldest first.
	Trend []int64
}

// FormEventRepository stores and aggregates funnel events. Queries mirror
// pg_analytics.go: GROUP BY date with sparse days, gap-filled by the caller.
type FormEventRepository interface {
	Insert(ctx context.Context, ev *models.FormEvent) *errx.Error
	Totals(ctx context.Context, orgID, formID uuid.UUID, from time.Time) (*models.FormStatsTotals, *errx.Error)
	DailySeries(ctx context.Context, orgID, formID uuid.UUID, from time.Time) ([]models.FormStatsDay, *errx.Error)
	// PageFunnel rolls up per-visitor progress; pages is the form's current
	// page count so the funnel always covers every page, visited or not.
	PageFunnel(ctx context.Context, orgID, formID uuid.UUID, from time.Time, pages int) ([]models.FormFunnelPage, *errx.Error)
	Breakdown(ctx context.Context, orgID, formID uuid.UUID, from time.Time, column string, limit int) ([]models.FormStatsBucket, *errx.Error)
	// CampaignBreakdown buckets views by the campaign that carried the
	// personalized link, keyed by campaign name.
	CampaignBreakdown(ctx context.Context, orgID, formID uuid.UUID, from time.Time, limit int) ([]models.FormStatsBucket, *errx.Error)
	RecentIdentified(ctx context.Context, orgID, formID uuid.UUID, from time.Time, limit int) ([]models.FormIdentifiedVisitor, *errx.Error)
	// ListAggregates rolls up starts/identified totals and a daily submit
	// trend (trendDays, oldest first) for every form of the org in one pass.
	ListAggregates(ctx context.Context, orgID uuid.UUID, trendDays int) (map[uuid.UUID]*FormAggregates, *errx.Error)
	// CampaignFormPerformance rolls up every form this campaign has handed a
	// personalized link to, with what recipients did with it.
	CampaignFormPerformance(ctx context.Context, orgID, campaignID uuid.UUID) ([]models.CampaignFormStats, *errx.Error)
	PruneBefore(ctx context.Context, before time.Time) (int64, *errx.Error)
}

type formEventRepository struct {
	DB *db.DB
}

func NewFormEventRepository(d *db.DB) FormEventRepository {
	return &formEventRepository{DB: d}
}

func (r *formEventRepository) Insert(ctx context.Context, ev *models.FormEvent) *errx.Error {
	_, err := r.DB.Exec(ctx, `
		INSERT INTO form_events (organization_id, form_id, event_type, visitor_key,
			page_index, pages_total, contact_id, campaign_id, referrer_domain, country_code, device)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, ev.OrganizationID, ev.FormID, ev.Type, ev.VisitorKey,
		ev.PageIndex, ev.PagesTotal, ev.ContactID, ev.CampaignID, ev.ReferrerDomain, ev.CountryCode, ev.Device)
	if err != nil {
		db.CaptureError(err, "form events insert", nil, "exec")
		return errx.InternalError()
	}
	return nil
}

func (r *formEventRepository) Totals(ctx context.Context, orgID, formID uuid.UUID, from time.Time) (*models.FormStatsTotals, *errx.Error) {
	var t models.FormStatsTotals
	err := r.DB.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type = 'view'),
		       COUNT(*) FILTER (WHERE event_type = 'start'),
		       COUNT(*) FILTER (WHERE event_type = 'submit'),
		       COUNT(DISTINCT contact_id) FILTER (WHERE contact_id IS NOT NULL)
		FROM form_events
		WHERE organization_id = $1 AND form_id = $2 AND occurred_at >= $3
	`, orgID, formID, from).Scan(&t.Views, &t.Starts, &t.Submissions, &t.IdentifiedVisitors)
	if err != nil {
		db.CaptureError(err, "form events totals", nil, "query")
		return nil, errx.InternalError()
	}
	if t.Starts > 0 {
		t.CompletionRate = float64(t.Submissions) / float64(t.Starts)
	}
	return &t, nil
}

func (r *formEventRepository) DailySeries(ctx context.Context, orgID, formID uuid.UUID, from time.Time) ([]models.FormStatsDay, *errx.Error) {
	rows, err := r.DB.Query(ctx, `
		SELECT occurred_at::date::text,
		       COUNT(*) FILTER (WHERE event_type = 'view'),
		       COUNT(*) FILTER (WHERE event_type = 'start'),
		       COUNT(*) FILTER (WHERE event_type = 'submit')
		FROM form_events
		WHERE organization_id = $1 AND form_id = $2 AND occurred_at >= $3
		GROUP BY occurred_at::date
		ORDER BY occurred_at::date ASC
	`, orgID, formID, from)
	if err != nil {
		db.CaptureError(err, "form events daily", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.FormStatsDay{}
	for rows.Next() {
		var d models.FormStatsDay
		if err := rows.Scan(&d.Date, &d.Views, &d.Starts, &d.Submissions); err != nil {
			db.CaptureError(err, "form events daily", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *formEventRepository) PageFunnel(ctx context.Context, orgID, formID uuid.UUID, from time.Time, pages int) ([]models.FormFunnelPage, *errx.Error) {
	if pages < 1 {
		pages = 1
	}
	rows, err := r.DB.Query(ctx, `
		WITH visitors AS (
			SELECT visitor_key,
			       COALESCE(MAX(page_index) FILTER (WHERE event_type = 'page'), 0) AS max_page,
			       BOOL_OR(event_type = 'submit') AS submitted
			FROM form_events
			WHERE organization_id = $1 AND form_id = $2 AND occurred_at >= $3 AND visitor_key <> ''
			GROUP BY visitor_key
		)
		SELECT gs.idx,
		       (SELECT COUNT(*) FROM visitors v WHERE v.max_page >= gs.idx),
		       (SELECT COUNT(*) FROM visitors v WHERE v.max_page >= gs.idx AND v.submitted)
		FROM generate_series(0, $4 - 1) AS gs(idx)
		ORDER BY gs.idx
	`, orgID, formID, from, pages)
	if err != nil {
		db.CaptureError(err, "form events funnel", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.FormFunnelPage{}
	for rows.Next() {
		var p models.FormFunnelPage
		if err := rows.Scan(&p.PageIndex, &p.Reached, &p.CompletedFrom); err != nil {
			db.CaptureError(err, "form events funnel", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *formEventRepository) Breakdown(ctx context.Context, orgID, formID uuid.UUID, from time.Time, column string, limit int) ([]models.FormStatsBucket, *errx.Error) {
	// The column is picked from a fixed set, never interpolated from input.
	var col string
	switch column {
	case "referrer_domain":
		col = "referrer_domain"
	case "country_code":
		col = "country_code"
	case "device":
		col = "device"
	default:
		return nil, errx.InternalError()
	}
	rows, err := r.DB.Query(ctx, `
		SELECT `+col+`, COUNT(*)
		FROM form_events
		WHERE organization_id = $1 AND form_id = $2 AND occurred_at >= $3
			AND event_type = 'view' AND `+col+` <> '' AND `+col+` <> 'unknown'
		GROUP BY `+col+`
		ORDER BY COUNT(*) DESC
		LIMIT $4
	`, orgID, formID, from, limit)
	if err != nil {
		db.CaptureError(err, "form events breakdown", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.FormStatsBucket{}
	for rows.Next() {
		var b models.FormStatsBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			db.CaptureError(err, "form events breakdown", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *formEventRepository) CampaignBreakdown(ctx context.Context, orgID, formID uuid.UUID, from time.Time, limit int) ([]models.FormStatsBucket, *errx.Error) {
	rows, err := r.DB.Query(ctx, `
		SELECT COALESCE(cp.name, 'Deleted campaign'), COUNT(*)
		FROM form_events e
		LEFT JOIN campaigns cp ON cp.id = e.campaign_id
		WHERE e.organization_id = $1 AND e.form_id = $2 AND e.occurred_at >= $3
			AND e.event_type = 'view' AND e.campaign_id IS NOT NULL
		GROUP BY cp.name
		ORDER BY COUNT(*) DESC
		LIMIT $4
	`, orgID, formID, from, limit)
	if err != nil {
		db.CaptureError(err, "form events campaigns", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.FormStatsBucket{}
	for rows.Next() {
		var b models.FormStatsBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			db.CaptureError(err, "form events campaigns", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *formEventRepository) RecentIdentified(ctx context.Context, orgID, formID uuid.UUID, from time.Time, limit int) ([]models.FormIdentifiedVisitor, *errx.Error) {
	rows, err := r.DB.Query(ctx, `
		SELECT e.contact_id,
		       COALESCE(NULLIF(TRIM(COALESCE(c.first_name, '') || ' ' || COALESCE(c.last_name, '')), ''), c.email),
		       c.email,
		       MAX(e.occurred_at),
		       COALESCE(MAX(e.page_index) FILTER (WHERE e.event_type = 'page'), 0),
		       BOOL_OR(e.event_type = 'submit'),
		       COALESCE(MAX(cp.name), '')
		FROM form_events e
		JOIN contacts c ON c.id = e.contact_id
		LEFT JOIN campaigns cp ON cp.id = e.campaign_id
		WHERE e.organization_id = $1 AND e.form_id = $2 AND e.occurred_at >= $3 AND e.contact_id IS NOT NULL
		GROUP BY e.contact_id, c.first_name, c.last_name, c.email
		ORDER BY MAX(e.occurred_at) DESC
		LIMIT $4
	`, orgID, formID, from, limit)
	if err != nil {
		db.CaptureError(err, "form events identified", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	out := []models.FormIdentifiedVisitor{}
	for rows.Next() {
		var v models.FormIdentifiedVisitor
		if err := rows.Scan(&v.ContactID, &v.Name, &v.Email, &v.LastSeen, &v.FurthestPage, &v.Completed, &v.Campaign); err != nil {
			db.CaptureError(err, "form events identified", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *formEventRepository) ListAggregates(ctx context.Context, orgID uuid.UUID, trendDays int) (map[uuid.UUID]*FormAggregates, *errx.Error) {
	out := map[uuid.UUID]*FormAggregates{}
	get := func(id uuid.UUID) *FormAggregates {
		if a, ok := out[id]; ok {
			return a
		}
		a := &FormAggregates{Trend: make([]int64, trendDays)}
		out[id] = a
		return a
	}

	rows, err := r.DB.Query(ctx, `
		SELECT form_id,
		       COUNT(*) FILTER (WHERE event_type = 'start'),
		       COUNT(DISTINCT contact_id) FILTER (WHERE contact_id IS NOT NULL)
		FROM form_events
		WHERE organization_id = $1
		GROUP BY form_id
	`, orgID)
	if err != nil {
		db.CaptureError(err, "form events aggregates", nil, "query")
		return nil, errx.InternalError()
	}
	for rows.Next() {
		var id uuid.UUID
		var starts, identified int64
		if err := rows.Scan(&id, &starts, &identified); err != nil {
			rows.Close()
			db.CaptureError(err, "form events aggregates", nil, "scan")
			return nil, errx.InternalError()
		}
		a := get(id)
		a.Starts, a.Identified = starts, identified
	}
	rows.Close()

	rows, err = r.DB.Query(ctx, `
		SELECT form_id, (occurred_at AT TIME ZONE 'UTC')::date, COUNT(*)
		FROM form_events
		WHERE organization_id = $1 AND event_type = 'submit'
			AND occurred_at >= NOW() - make_interval(days => $2)
		GROUP BY form_id, (occurred_at AT TIME ZONE 'UTC')::date
	`, orgID, trendDays)
	if err != nil {
		db.CaptureError(err, "form events trend", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for rows.Next() {
		var id uuid.UUID
		var day time.Time
		var count int64
		if err := rows.Scan(&id, &day, &count); err != nil {
			db.CaptureError(err, "form events trend", nil, "scan")
			return nil, errx.InternalError()
		}
		slot := trendDays - 1 - int(today.Sub(day.Truncate(24*time.Hour)).Hours()/24)
		if slot >= 0 && slot < trendDays {
			get(id).Trend[slot] = count
		}
	}
	return out, nil
}

func (r *formEventRepository) CampaignFormPerformance(ctx context.Context, orgID, campaignID uuid.UUID) ([]models.CampaignFormStats, *errx.Error) {
	rows, err := r.DB.Query(ctx, `
		WITH links AS (
			SELECT form_id, COUNT(*) AS links_sent
			FROM form_links
			WHERE organization_id = $1 AND campaign_id = $2
			GROUP BY form_id
		), ev AS (
			SELECT form_id,
				COUNT(DISTINCT visitor_key) FILTER (WHERE event_type = 'view') AS viewers,
				COUNT(DISTINCT visitor_key) FILTER (WHERE event_type = 'start') AS starters
			FROM form_events
			WHERE organization_id = $1 AND campaign_id = $2
			GROUP BY form_id
		), sub AS (
			SELECT form_id, COUNT(*) AS submissions
			FROM form_submissions
			WHERE organization_id = $1 AND campaign_id = $2
			GROUP BY form_id
		), refd AS (
			-- Forms written into a step but not sent yet still belong in the
			-- panel, at zero. Keep the pattern in sync with FormLinkMarkerRE.
			SELECT DISTINCT m[1] AS public_id
			FROM sequences q,
				regexp_matches(COALESCE(q.subject, '') || ' ' || COALESCE(q.body_html, ''),
					'\{\{\s*form_link:([a-z0-9]{1,64})\s*\}\}', 'g') AS m
			WHERE q.campaign_id = $2 AND q.organization_id = $1
		)
		SELECT f.id, f.name, f.public_id, f.status,
			COALESCE(l.links_sent, 0), COALESCE(e.viewers, 0),
			COALESCE(e.starters, 0), COALESCE(s.submissions, 0)
		FROM forms f
		LEFT JOIN links l ON l.form_id = f.id
		LEFT JOIN ev e ON e.form_id = f.id
		LEFT JOIN sub s ON s.form_id = f.id
		WHERE f.organization_id = $1
			AND (l.form_id IS NOT NULL OR e.form_id IS NOT NULL OR s.form_id IS NOT NULL
				OR f.public_id IN (SELECT public_id FROM refd))
		ORDER BY COALESCE(s.submissions, 0) DESC, COALESCE(l.links_sent, 0) DESC, f.name
	`, orgID, campaignID)
	if err != nil {
		db.CaptureError(err, "campaign form performance", nil, "query")
		return nil, errx.InternalError()
	}
	defer rows.Close()

	out := []models.CampaignFormStats{}
	for rows.Next() {
		var s models.CampaignFormStats
		if err := rows.Scan(&s.FormID, &s.FormName, &s.PublicID, &s.Status,
			&s.LinksSent, &s.Viewers, &s.Starters, &s.Submissions); err != nil {
			db.CaptureError(err, "campaign form performance", nil, "scan")
			return nil, errx.InternalError()
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *formEventRepository) PruneBefore(ctx context.Context, before time.Time) (int64, *errx.Error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM form_events WHERE occurred_at < $1`, before)
	if err != nil {
		db.CaptureError(err, "form events prune", nil, "exec")
		return 0, errx.InternalError()
	}
	return tag.RowsAffected(), nil
}
