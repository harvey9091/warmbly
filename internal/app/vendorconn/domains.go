package vendorconn

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/mailvendor"
)

// domainListTTL keeps one vendor's domain list for a while; the domains page reads it on every open.
const domainListTTL = 5 * time.Minute

// noDNSWrites are vendors whose DNS API replaces a domain's whole record set;
// one bad write could drop its mail records, so only forwarding is used there.
var noDNSWrites = map[string]bool{mailvendor.VendorMailforge: true, mailvendor.VendorInfraforge: true}

type domainList struct {
	domains []mailvendor.Domain
	at      time.Time
}

type domainCache struct {
	mu   sync.Mutex
	byID map[uuid.UUID]domainList
}

// vendorDomain is one domain with the connection that holds it.
type vendorDomain struct {
	conn   models.VendorConnection
	client mailvendor.Client
	mgr    mailvendor.DomainManager
	domain mailvendor.Domain
}

func (s *Service) domainsOf(ctx context.Context, c *models.VendorConnection) ([]mailvendor.Domain, mailvendor.DomainManager, *errx.Error) {
	client, xerr := s.client(ctx, c)
	if xerr != nil {
		return nil, nil, xerr
	}
	mgr, ok := client.(mailvendor.DomainManager)
	if !ok {
		return nil, nil, nil
	}
	s.domainCache.mu.Lock()
	if s.domainCache.byID == nil {
		s.domainCache.byID = map[uuid.UUID]domainList{}
	}
	if l, ok := s.domainCache.byID[c.ID]; ok && time.Since(l.at) < domainListTTL {
		s.domainCache.mu.Unlock()
		return l.domains, mgr, nil
	}
	s.domainCache.mu.Unlock()
	list, err := mgr.Domains(ctx)
	if err != nil {
		return nil, nil, s.failed(ctx, c, err)
	}
	s.domainCache.mu.Lock()
	s.domainCache.byID[c.ID] = domainList{domains: list, at: time.Now()}
	s.domainCache.mu.Unlock()
	return list, mgr, nil
}

func (s *Service) forgetDomains(id uuid.UUID) {
	s.domainCache.mu.Lock()
	delete(s.domainCache.byID, id)
	s.domainCache.mu.Unlock()
}

func capabilities(vendor string, mgr mailvendor.DomainManager) mailvendor.DomainCapabilities {
	c := mgr.DomainCapabilities()
	if noDNSWrites[vendor] {
		c.DNS, c.DNSTypes = false, nil
	}
	return c
}

// DomainLinks maps every domain the workspace's vendor accounts hold to what
// the vendor's API can do for it. A vendor that cannot be reached is skipped.
func (s *Service) DomainLinks(ctx context.Context, orgID uuid.UUID) (map[string]models.VendorDomainLink, *errx.Error) {
	conns, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, errx.InternalError()
	}
	out := map[string]models.VendorDomainLink{}
	for _, lc := range conns {
		if lc.Status != "active" {
			continue
		}
		c, xerr := s.get(ctx, orgID, lc.ID)
		if xerr != nil {
			continue
		}
		domains, mgr, xerr := s.domainsOf(ctx, c)
		if xerr != nil || mgr == nil {
			continue
		}
		caps := capabilities(c.Vendor, mgr)
		for _, d := range domains {
			name := strings.ToLower(strings.TrimSuffix(d.Name, "."))
			if name == "" {
				continue
			}
			out[name] = models.VendorDomainLink{
				Vendor: c.Vendor, ConnectionID: c.ID, Forwarding: d.Forwarding,
				CanForward: caps.Forwarding, CanUnforward: caps.ForwardingRemove, ForwardingReviewed: caps.ForwardingReviewed,
				CanDNS: caps.DNS, DNSTypes: caps.DNSTypes,
			}
		}
	}
	return out, nil
}

// find is the workspace's vendor account holding a domain.
func (s *Service) find(ctx context.Context, orgID uuid.UUID, domain string) (*vendorDomain, *errx.Error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	conns, err := s.repo.List(ctx, orgID)
	if err != nil {
		return nil, errx.InternalError()
	}
	for _, lc := range conns {
		if lc.Status != "active" {
			continue
		}
		c, xerr := s.get(ctx, orgID, lc.ID)
		if xerr != nil {
			continue
		}
		domains, mgr, xerr := s.domainsOf(ctx, c)
		if xerr != nil || mgr == nil {
			continue
		}
		for _, d := range domains {
			if strings.EqualFold(strings.TrimSuffix(d.Name, "."), domain) {
				client, _ := s.client(ctx, c)
				return &vendorDomain{conn: *c, client: client, mgr: mgr, domain: d}, nil
			}
		}
	}
	return nil, errx.NewWithIdentifier(errx.NotFound, ErrIDNoVendorDomain, "None of the workspace's vendor accounts holds this domain.")
}

// ErrIDNoVendorDomain and ErrIDVendorCannot are refusals a client branches on.
const (
	ErrIDNoVendorDomain = "mailbox_vendor_domain_not_found"
	ErrIDVendorCannot   = "mailbox_vendor_domain_unsupported"
)

// SetForwarding points the domain's root at a website through the vendor that holds it.
func (s *Service) SetForwarding(ctx context.Context, orgID uuid.UUID, domain, target string) (*models.VendorDomainLink, *errx.Error) {
	vd, xerr := s.find(ctx, orgID, domain)
	if xerr != nil {
		return nil, xerr
	}
	caps := capabilities(vd.conn.Vendor, vd.mgr)
	if !caps.Forwarding || (target == "" && !caps.ForwardingRemove) {
		return nil, errx.NewWithIdentifier(errx.BadRequest, ErrIDVendorCannot, labelOf(vd.conn.Vendor)+" cannot do this for a domain through its API.")
	}
	if err := vd.mgr.SetForwarding(ctx, vd.domain, target); err != nil {
		if x := refusedChange(vd.conn.Vendor, err); x != nil {
			return nil, x
		}
		return nil, s.failed(ctx, &vd.conn, err)
	}
	s.forgetDomains(vd.conn.ID)
	link := models.VendorDomainLink{
		Vendor: vd.conn.Vendor, ConnectionID: vd.conn.ID, Forwarding: target,
		CanForward: caps.Forwarding, CanUnforward: caps.ForwardingRemove, ForwardingReviewed: caps.ForwardingReviewed,
		CanDNS: caps.DNS, DNSTypes: caps.DNSTypes,
	}
	return &link, nil
}

// UpsertDNS writes one record on the domain through the vendor that holds it.
func (s *Service) UpsertDNS(ctx context.Context, orgID uuid.UUID, domain, typ, name, value string) *errx.Error {
	vd, xerr := s.find(ctx, orgID, domain)
	if xerr != nil {
		return xerr
	}
	caps := capabilities(vd.conn.Vendor, vd.mgr)
	allowed := false
	for _, t := range caps.DNSTypes {
		allowed = allowed || strings.EqualFold(t, typ)
	}
	if !caps.DNS || !allowed {
		return errx.NewWithIdentifier(errx.BadRequest, ErrIDVendorCannot, labelOf(vd.conn.Vendor)+" cannot write this DNS record through its API.")
	}
	if err := vd.mgr.UpsertDNSRecord(ctx, vd.domain, mailvendor.DNSRecord{Type: strings.ToUpper(typ), Name: name, Value: value, TTL: 3600}); err != nil {
		if x := refusedChange(vd.conn.Vendor, err); x != nil {
			return x
		}
		return s.failed(ctx, &vd.conn, err)
	}
	return nil
}

// refusedChange is a change the vendor client refused before sending it. Its
// reason is written by mailvendor, never by the vendor, so it is safe to show.
func refusedChange(vendor string, err error) *errx.Error {
	if !errors.Is(err, mailvendor.ErrInvalidConfig) {
		return nil
	}
	reason := err.Error()
	if i := strings.LastIndex(reason, ": "); i >= 0 {
		reason = reason[i+2:]
	}
	return errx.NewWithIdentifier(errx.BadRequest, ErrIDVendorCannot, labelOf(vendor)+" cannot take this change: "+reason+".")
}
