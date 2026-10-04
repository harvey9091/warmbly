package mailvendor

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Cheap Inboxes: https://api.cheapinboxes.com/v1/openapi.json
type cheapInboxes struct {
	t *transport
}

const (
	cheapInboxesPageSize = 100
	// cheapInboxesPerSecond keeps under the documented 120 requests a minute.
	cheapInboxesPerSecond = 2
)

func newCheapInboxes(vals map[string]string, o options) *cheapInboxes {
	return &cheapInboxes{t: newTransport(VendorCheapInboxes, "https://api.cheapinboxes.com", o, bearer(vals[FieldAPIKey]), cheapInboxesPerSecond)}
}

func (c *cheapInboxes) Vendor() string { return VendorCheapInboxes }

// Verify reads the organization, which the docs name as the key check.
func (c *cheapInboxes) Verify(ctx context.Context) error {
	return c.t.do(ctx, call{method: http.MethodGet, path: "/v1/org"}, nil)
}

func (c *cheapInboxes) List(ctx context.Context) ([]Mailbox, error) {
	var out []Mailbox
	for offset, page := 0, 0; page < maxPages; page++ {
		var res struct {
			Mailboxes []struct {
				ID             string `json:"id"`
				FullEmail      string `json:"full_email"`
				FirstName      string `json:"first_name"`
				LastName       string `json:"last_name"`
				Status         string `json:"status"`
				SourceProvider string `json:"source_provider"`
			} `json:"mailboxes"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		q := url.Values{"limit": {strconv.Itoa(cheapInboxesPageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := c.t.do(ctx, call{method: http.MethodGet, path: "/v1/mailboxes", query: q}, &res); err != nil {
			return nil, err
		}
		for _, m := range res.Mailboxes {
			out = append(out, Mailbox{
				ID:        m.ID,
				Email:     m.FullEmail,
				FirstName: m.FirstName,
				LastName:  m.LastName,
				Domain:    domainOf(m.FullEmail),
				Provider:  normalizeProvider(m.SourceProvider),
				Status:    m.Status,
			})
			if len(out) >= MaxMailboxes {
				return out, nil
			}
		}
		offset += len(res.Mailboxes)
		if len(res.Mailboxes) == 0 || offset >= res.Pagination.Total {
			break
		}
	}
	return out, nil
}

// Credentials reads the credentials endpoint; the list carries none, and only active mailboxes have them.
func (c *cheapInboxes) Credentials(ctx context.Context, m Mailbox) (Credentials, error) {
	if m.ID == "" {
		return Credentials{}, vendorErr(VendorCheapInboxes, 0, "mailbox id is required", ErrNotFound)
	}
	var res struct {
		Credentials *struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			AppPassword string `json:"app_password"`
			IMAPHost    string `json:"imap_host"`
			IMAPPort    int    `json:"imap_port"`
			SMTPHost    string `json:"smtp_host"`
			SMTPPort    int    `json:"smtp_port"`
		} `json:"credentials"`
	}
	path := "/v1/mailboxes/" + url.PathEscape(m.ID) + "/credentials"
	if err := c.t.do(ctx, call{method: http.MethodGet, path: path}, &res); err != nil {
		return Credentials{}, err
	}
	k := res.Credentials
	if k == nil {
		return Credentials{}, vendorErr(VendorCheapInboxes, http.StatusOK, "no credentials returned", nil)
	}
	user := firstNonEmpty(k.Email, m.Email)
	return Credentials{
		Password:    k.Password,
		AppPassword: k.AppPassword,
		SMTP:        endpoint(k.SMTPHost, k.SMTPPort, user),
		IMAP:        endpoint(k.IMAPHost, k.IMAPPort, user),
	}, nil
}

func (c *cheapInboxes) DomainCapabilities() DomainCapabilities {
	return DomainCapabilities{Forwarding: true, ForwardingRemove: true, DNS: true, DNSTypes: allDNSTypes()}
}

// Domains pages through GET /v1/domains.
func (c *cheapInboxes) Domains(ctx context.Context) ([]Domain, error) {
	var out []Domain
	for offset, page := 0, 0; page < maxPages; page++ {
		var res struct {
			Domains []struct {
				ID            string  `json:"id"`
				Domain        string  `json:"domain"`
				ForwardingURL *string `json:"forwarding_url"`
			} `json:"domains"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		q := url.Values{"limit": {strconv.Itoa(cheapInboxesPageSize)}, "offset": {strconv.Itoa(offset)}}
		if err := c.t.do(ctx, call{method: http.MethodGet, path: "/v1/domains", query: q}, &res); err != nil {
			return nil, err
		}
		for _, d := range res.Domains {
			dom := Domain{ID: d.ID, Name: d.Domain}
			if d.ForwardingURL != nil {
				dom.Forwarding = *d.ForwardingURL
			}
			out = append(out, dom)
			if len(out) >= MaxDomains {
				return out, nil
			}
		}
		offset += len(res.Domains)
		if len(res.Domains) == 0 || offset >= res.Pagination.Total {
			break
		}
	}
	return out, nil
}

// SetForwarding calls PATCH /v1/domains/{id}/forwarding; null clears it. permanent is omitted, so the vendor's 302 applies.
func (c *cheapInboxes) SetForwarding(ctx context.Context, d Domain, target string) error {
	target, err := forwardingTarget(VendorCheapInboxes, target, c.DomainCapabilities())
	if err != nil {
		return err
	}
	if d.ID == "" {
		return invalid(VendorCheapInboxes, "domain id is required")
	}
	var fwd *string
	if target != "" {
		fwd = &target
	}
	path := "/v1/domains/" + url.PathEscape(d.ID) + "/forwarding"
	return c.t.do(ctx, call{method: http.MethodPatch, path: path, body: map[string]any{"forwarding_url": fwd}}, nil)
}

type cheapInboxesRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     *int   `json:"ttl,omitempty"`
}

// UpsertDNSRecord lists /v1/domains/{id}/dns-records of the record's type, then POSTs, PATCHes or DELETEs.
// The vendor refuses changes to the records it manages (default MX, SPF, DKIM).
func (c *cheapInboxes) UpsertDNSRecord(ctx context.Context, d Domain, r DNSRecord) error {
	r, domain, err := prepareRecord(VendorCheapInboxes, d, r, c.DomainCapabilities())
	if err != nil {
		return err
	}
	if d.ID == "" {
		return invalid(VendorCheapInboxes, "domain id is required")
	}
	base := "/v1/domains/" + url.PathEscape(d.ID) + "/dns-records"
	var list struct {
		Records []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"records"`
	}
	if err := c.t.do(ctx, call{method: http.MethodGet, path: base, query: url.Values{"type": {r.Type}}}, &list); err != nil {
		return err
	}
	existing := make([]existingRecord, len(list.Records))
	for i, e := range list.Records {
		existing[i] = existingRecord{ID: e.ID, Type: e.Type, Name: absoluteHost(e.Name, domain), Value: e.Content}
	}
	p := planUpsert(existing, r)
	if p.noop {
		return nil
	}
	if err := needIDs(VendorCheapInboxes, existing, p); err != nil {
		return err
	}
	rec := cheapInboxesRecord{Type: r.Type, Name: relativeHost(r.Name, domain), Content: r.Value, TTL: ttlOrNil(r.TTL)}
	if p.update >= 0 {
		err = c.t.do(ctx, call{method: http.MethodPatch, path: base + "/" + url.PathEscape(existing[p.update].ID), body: rec}, nil)
	} else {
		err = c.t.do(ctx, call{method: http.MethodPost, path: base, body: rec}, nil)
	}
	if err != nil {
		return err
	}
	for _, idx := range p.remove {
		if err := c.t.do(ctx, call{method: http.MethodDelete, path: base + "/" + url.PathEscape(existing[idx].ID)}, nil); err != nil {
			return err
		}
	}
	return nil
}
