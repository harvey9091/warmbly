package mailvendor

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// InboxKit: https://docs.inboxkit.com/llms.txt
type inboxKit struct {
	t *transport
}

const inboxKitPageSize = 100

func newInboxKit(vals map[string]string, o options) *inboxKit {
	key, ws := vals[FieldAPIKey], vals[FieldWorkspaceID]
	auth := func(h http.Header) {
		h.Set("Authorization", "Bearer "+key)
		h.Set("X-Workspace-Id", ws)
	}
	return &inboxKit{t: newTransport(VendorInboxKit, "https://api.inboxkit.com", o, auth, 0)}
}

func (c *inboxKit) Vendor() string { return VendorInboxKit }

type inboxKitList struct {
	Error     bool `json:"error"`
	Mailboxes []struct {
		UID        string `json:"uid"`
		DomainName string `json:"domain_name"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		Username   string `json:"username"`
		Platform   string `json:"platform"`
		Status     string `json:"status"`
	} `json:"mailboxes"`
	Pages       int `json:"pages"`
	CurrentPage int `json:"current_page"`
}

func (c *inboxKit) page(ctx context.Context, page, limit int) (inboxKitList, error) {
	var out inboxKitList
	body := map[string]any{"page": page, "limit": limit}
	if err := c.t.do(ctx, call{method: http.MethodPost, path: "/v1/api/mailboxes/list", body: body}, &out); err != nil {
		return out, err
	}
	if out.Error {
		return out, vendorErr(VendorInboxKit, http.StatusOK, "vendor reported an error", nil)
	}
	return out, nil
}

// Verify lists one mailbox, which checks the key and the workspace together.
func (c *inboxKit) Verify(ctx context.Context) error {
	_, err := c.page(ctx, 1, 1)
	return err
}

func (c *inboxKit) List(ctx context.Context) ([]Mailbox, error) {
	var out []Mailbox
	for page := 1; page <= maxPages; page++ {
		res, err := c.page(ctx, page, inboxKitPageSize)
		if err != nil {
			return nil, err
		}
		for _, m := range res.Mailboxes {
			email := m.Username
			if m.DomainName != "" {
				email = m.Username + "@" + m.DomainName
			}
			out = append(out, Mailbox{
				ID:        m.UID,
				Email:     email,
				FirstName: m.FirstName,
				LastName:  m.LastName,
				Domain:    m.DomainName,
				Provider:  normalizeProvider(m.Platform),
				Status:    m.Status,
			})
			if len(out) >= MaxMailboxes {
				return out, nil
			}
		}
		if len(res.Mailboxes) == 0 || page >= res.Pages {
			break
		}
	}
	return out, nil
}

// Credentials reads show-credentials; InboxKit's list carries none.
func (c *inboxKit) Credentials(ctx context.Context, m Mailbox) (Credentials, error) {
	q := url.Values{}
	if m.ID != "" {
		q.Set("uid", m.ID)
	} else {
		q.Set("email", m.Email)
	}
	var res struct {
		Error       bool   `json:"error"`
		Password    string `json:"password"`
		AppPassword string `json:"app_password"`
	}
	if err := c.t.do(ctx, call{method: http.MethodGet, path: "/v1/api/mailboxes/show-credentials", query: q}, &res); err != nil {
		return Credentials{}, err
	}
	if res.Error {
		return Credentials{}, vendorErr(VendorInboxKit, http.StatusOK, "vendor reported an error", nil)
	}
	return Credentials{Password: res.Password, AppPassword: res.AppPassword}, nil
}

const inboxKitDomainPageSize = 100

// inboxKitEnvelope carries InboxKit's error flag, which can be set inside an HTTP 200.
type inboxKitEnvelope struct {
	Error bool `json:"error"`
}

func (e inboxKitEnvelope) err() error {
	if e.Error {
		return vendorErr(VendorInboxKit, http.StatusOK, "vendor reported an error", nil)
	}
	return nil
}

func (c *inboxKit) DomainCapabilities() DomainCapabilities {
	return DomainCapabilities{Forwarding: true, ForwardingRemove: true, DNS: true, DNSTypes: allDNSTypes()}
}

// Domains pages through POST /v1/api/domains/list.
func (c *inboxKit) Domains(ctx context.Context) ([]Domain, error) {
	var out []Domain
	for page := 1; page <= maxPages; page++ {
		var res struct {
			inboxKitEnvelope
			Domains []struct {
				UID           string `json:"uid"`
				Name          string `json:"name"`
				TLD           string `json:"tld"`
				ForwardingURL string `json:"forwarding_url"`
			} `json:"domains"`
			Pages int `json:"pages"`
		}
		body := map[string]any{"page": page, "limit": inboxKitDomainPageSize}
		if err := c.t.do(ctx, call{method: http.MethodPost, path: "/v1/api/domains/list", body: body}, &res); err != nil {
			return nil, err
		}
		if err := res.err(); err != nil {
			return nil, err
		}
		for _, d := range res.Domains {
			name := d.Name
			// The Domain schema describes name as the label without its TLD; the list example carries both.
			if name != "" && !strings.Contains(name, ".") && d.TLD != "" {
				name += "." + strings.TrimPrefix(d.TLD, ".")
			}
			out = append(out, Domain{ID: d.UID, Name: name, Forwarding: d.ForwardingURL})
			if len(out) >= MaxDomains {
				return out, nil
			}
		}
		if len(res.Domains) == 0 || page >= res.Pages {
			break
		}
	}
	return out, nil
}

// SetForwarding calls POST /v1/api/domains/forwarding; an empty forwarding_url removes it.
func (c *inboxKit) SetForwarding(ctx context.Context, d Domain, target string) error {
	target, err := forwardingTarget(VendorInboxKit, target, c.DomainCapabilities())
	if err != nil {
		return err
	}
	if d.ID == "" {
		return invalid(VendorInboxKit, "domain id is required")
	}
	var res inboxKitEnvelope
	body := map[string]any{"uids": []string{d.ID}, "forwarding_url": target}
	if err := c.t.do(ctx, call{method: http.MethodPost, path: "/v1/api/domains/forwarding", body: body}, &res); err != nil {
		return err
	}
	return res.err()
}

type inboxKitRecord struct {
	// ID is not in the documented list schema; record ids are the store's _id, which update and delete take.
	ID       string `json:"_id,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	Host     string `json:"host"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      *int   `json:"ttl,omitempty"`
}

// inboxKitTarget names the domain by uid, else by name; the DNS endpoints take either.
func inboxKitTarget(d Domain, domain string) map[string]any {
	if d.ID != "" {
		return map[string]any{"uid": d.ID}
	}
	return map[string]any{"domain": domain}
}

// UpsertDNSRecord reads /v1/api/dns/list, then adds, updates or deletes through /v1/api/dns/{add,update,delete}.
func (c *inboxKit) UpsertDNSRecord(ctx context.Context, d Domain, r DNSRecord) error {
	r, domain, err := prepareRecord(VendorInboxKit, d, r, c.DomainCapabilities())
	if err != nil {
		return err
	}
	q := url.Values{}
	if d.ID != "" {
		q.Set("uid", d.ID)
	} else {
		q.Set("domain", domain)
	}
	var list struct {
		inboxKitEnvelope
		DNSRecord struct {
			Records []inboxKitRecord `json:"records"`
		} `json:"dns_record"`
	}
	err = c.t.do(ctx, call{method: http.MethodGet, path: "/v1/api/dns/list", query: q}, &list)
	// A 404 also means "no records yet"; a missing domain fails again on the write.
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil {
		if err := list.err(); err != nil {
			return err
		}
	}
	existing := make([]existingRecord, len(list.DNSRecord.Records))
	for i, e := range list.DNSRecord.Records {
		existing[i] = existingRecord{ID: firstNonEmpty(e.ID, e.RecordID), Type: e.Type, Name: absoluteHost(e.Host, domain), Value: e.Value}
	}
	p := planUpsert(existing, r)
	if p.noop {
		return nil
	}
	if err := needIDs(VendorInboxKit, existing, p); err != nil {
		return err
	}
	rec := inboxKitRecord{Host: relativeHost(r.Name, domain), Type: r.Type, Value: r.Value, TTL: ttlOrNil(r.TTL)}
	path := "/v1/api/dns/add"
	if p.update >= 0 {
		path = "/v1/api/dns/update"
		rec.RecordID = existing[p.update].ID
	}
	body := inboxKitTarget(d, domain)
	body["records"] = []inboxKitRecord{rec}
	var res inboxKitEnvelope
	if err := c.t.do(ctx, call{method: http.MethodPost, path: path, body: body}, &res); err != nil {
		return err
	}
	if err := res.err(); err != nil {
		return err
	}
	if len(p.remove) == 0 {
		return nil
	}
	ids := make([]string, len(p.remove))
	for i, idx := range p.remove {
		ids[i] = existing[idx].ID
	}
	del := inboxKitTarget(d, domain)
	del["record_ids"] = ids
	res = inboxKitEnvelope{}
	if err := c.t.do(ctx, call{method: http.MethodPost, path: "/v1/api/dns/delete", body: del}, &res); err != nil {
		return err
	}
	return res.err()
}
