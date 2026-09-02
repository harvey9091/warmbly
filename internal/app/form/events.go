package form

import (
	"context"
	"net/netip"
	"net/url"
	"strings"

	"github.com/mileusna/useragent"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// EventInput is one funnel event as the forms service forwards it. RemoteIP
// feeds the country lookup and is discarded; it is never stored.
type EventInput struct {
	Type       string
	PageIndex  int
	PagesTotal int
	VisitorKey string
	SourceURL  string
	LinkToken  string
	RemoteIP   string
	UserAgent  string
}

func (s *service) RecordEvent(ctx context.Context, publicID string, in EventInput) *errx.Error {
	f, xerr := s.PublicForm(ctx, publicID)
	if xerr != nil {
		return xerr
	}
	if s.events == nil {
		return nil
	}
	t := models.FormEventType(in.Type)
	if !t.Valid() || t == models.FormEventSubmit {
		// Submits are recorded by the submit pipeline, never by the client.
		return errx.New(errx.BadRequest, "unknown event type")
	}

	ev := &models.FormEvent{
		OrganizationID: f.OrganizationID,
		FormID:         f.ID,
		Type:           t,
		VisitorKey:     truncate(in.VisitorKey, 64),
		PageIndex:      clampInt(in.PageIndex, 0, 200),
		PagesTotal:     clampInt(in.PagesTotal, 0, 200),
		ReferrerDomain: referrerDomain(in.SourceURL),
		CountryCode:    s.countryFor(in.RemoteIP),
		Device:         deviceFor(in.UserAgent),
	}
	if link, _ := s.ResolveLink(ctx, f, in.LinkToken); link != nil {
		id := link.ContactID
		ev.ContactID = &id
		ev.CampaignID = link.CampaignID
	}
	if xerr := s.events.Insert(ctx, ev); xerr != nil {
		return xerr
	}
	if t == models.FormEventView {
		s.RecordView(ctx, f.ID)
	}
	return nil
}

// countryFor resolves the country only; private and loopback sources are
// skipped, and the IP is dropped right after the lookup.
func (s *service) countryFor(ip string) string {
	if s.geo == nil || ip == "" {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || addr.IsPrivate() || addr.IsLoopback() {
		return ""
	}
	info, err := s.geo.Lookup(addr)
	if err != nil || info == nil {
		return ""
	}
	return info.CountryCode
}

func deviceFor(ua string) string {
	if ua == "" {
		return "unknown"
	}
	parsed := useragent.Parse(ua)
	switch {
	case parsed.Tablet:
		return "tablet"
	case parsed.Mobile:
		return "mobile"
	case parsed.Desktop:
		return "desktop"
	default:
		return "unknown"
	}
}

func referrerDomain(sourceURL string) string {
	u, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	return truncate(strings.TrimPrefix(host, "www."), 253)
}

// formPageCount is how many pages the field list renders as.
func formPageCount(fields []models.FormField) int {
	pages := 1
	for _, f := range fields {
		if f.Type == models.FormFieldPageBreak {
			pages++
		}
	}
	return pages
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
