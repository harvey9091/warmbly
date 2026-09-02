package form

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

const (
	statsBucketLimit     = 8
	statsIdentifiedLimit = 25
)

func (s *service) Stats(ctx context.Context, orgID, formID uuid.UUID, rangeDays int) (*models.FormStats, *errx.Error) {
	switch rangeDays {
	case 7, 30, 90:
	default:
		return nil, errx.New(errx.BadRequest, "range must be 7d, 30d or 90d")
	}
	f, xerr := s.repo.Get(ctx, orgID, formID)
	if xerr != nil {
		return nil, xerr
	}
	if s.events == nil {
		return nil, errx.InternalError()
	}
	from := time.Now().AddDate(0, 0, -rangeDays)

	totals, xerr := s.events.Totals(ctx, orgID, formID, from)
	if xerr != nil {
		return nil, xerr
	}
	daily, xerr := s.events.DailySeries(ctx, orgID, formID, from)
	if xerr != nil {
		return nil, xerr
	}
	funnel, xerr := s.events.PageFunnel(ctx, orgID, formID, from, formPageCount(f.Fields))
	if xerr != nil {
		return nil, xerr
	}
	stitchPageTitles(f.Fields, funnel)
	sources, xerr := s.events.Breakdown(ctx, orgID, formID, from, "referrer_domain", statsBucketLimit)
	if xerr != nil {
		return nil, xerr
	}
	countries, xerr := s.events.Breakdown(ctx, orgID, formID, from, "country_code", statsBucketLimit)
	if xerr != nil {
		return nil, xerr
	}
	devices, xerr := s.events.Breakdown(ctx, orgID, formID, from, "device", statsBucketLimit)
	if xerr != nil {
		return nil, xerr
	}
	campaigns, xerr := s.events.CampaignBreakdown(ctx, orgID, formID, from, statsBucketLimit)
	if xerr != nil {
		return nil, xerr
	}
	identified, xerr := s.events.RecentIdentified(ctx, orgID, formID, from, statsIdentifiedLimit)
	if xerr != nil {
		return nil, xerr
	}

	return &models.FormStats{
		Totals:     *totals,
		Daily:      daily,
		Pages:      funnel,
		Sources:    sources,
		Countries:  countries,
		Devices:    devices,
		Campaigns:  campaigns,
		Identified: identified,
	}, nil
}

// stitchPageTitles labels funnel rows from the form's page breaks: page 0 is
// the opening page, every later page takes its break's label.
func stitchPageTitles(fields []models.FormField, funnel []models.FormFunnelPage) {
	titles := []string{""}
	for _, f := range fields {
		if f.Type == models.FormFieldPageBreak {
			titles = append(titles, f.Label)
		}
	}
	for i := range funnel {
		idx := funnel[i].PageIndex
		title := ""
		if idx >= 0 && idx < len(titles) {
			title = titles[idx]
		}
		if title == "" {
			title = fmt.Sprintf("Page %d", idx+1)
		}
		funnel[i].Title = title
	}
}

// CampaignForms answers the campaign side of the same funnel: per form, how
// many recipients got a personalized link and how far they went.
func (s *service) CampaignForms(ctx context.Context, orgID, campaignID uuid.UUID) ([]models.CampaignFormStats, *errx.Error) {
	if s.events == nil {
		return nil, errx.InternalError()
	}
	rows, xerr := s.events.CampaignFormPerformance(ctx, orgID, campaignID)
	if xerr != nil {
		return nil, xerr
	}
	host := s.FormsHost(ctx, orgID)
	for i := range rows {
		rows[i].ShareURL = config.FormURLOn(host, rows[i].PublicID)
	}
	return rows, nil
}
