package mailvendor

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Maildoso: https://developers.maildoso.com/openapi.json
type maildoso struct {
	t     *transport
	cache credCache
}

const maildosoPageSize = 500

func newMaildoso(vals map[string]string, o options) *maildoso {
	return &maildoso{t: newTransport(VendorMaildoso, "https://api.maildoso.com", o, bearer(vals[FieldAPIKey]), 0)}
}

func (c *maildoso) Vendor() string { return VendorMaildoso }

type maildosoAccount struct {
	ID        int     `json:"id"`
	Email     string  `json:"email_account"`
	Password  string  `json:"password"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Provider  string  `json:"provider"`
	Status    string  `json:"status"`
	IMAP      *struct {
		Host string `json:"imap_host"`
		Port int    `json:"port"`
	} `json:"imap"`
	SMTP *struct {
		Host string `json:"smtp_host"`
		Port int    `json:"port"`
	} `json:"smtp"`
}

type maildosoPage struct {
	Items []maildosoAccount `json:"items"`
	Meta  struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func (a maildosoAccount) credentials() Credentials {
	cr := Credentials{Password: a.Password}
	// Both are null for Google accounts.
	if a.SMTP != nil {
		cr.SMTP = endpoint(a.SMTP.Host, a.SMTP.Port, a.Email)
	}
	if a.IMAP != nil {
		cr.IMAP = endpoint(a.IMAP.Host, a.IMAP.Port, a.Email)
	}
	return cr
}

func (a maildosoAccount) mailbox() Mailbox {
	m := Mailbox{
		ID:       strconv.Itoa(a.ID),
		Email:    a.Email,
		Domain:   domainOf(a.Email),
		Provider: normalizeProvider(a.Provider),
		Status:   a.Status,
	}
	if a.FirstName != nil {
		m.FirstName = *a.FirstName
	}
	if a.LastName != nil {
		m.LastName = *a.LastName
	}
	return m
}

func (c *maildoso) lookup(ctx context.Context, q url.Values) (maildosoPage, error) {
	var res maildosoPage
	err := c.t.do(ctx, call{method: http.MethodGet, path: "/v1/user/accounts-lookup", query: q}, &res)
	return res, err
}

// Verify asks for one account in the lightweight projection.
func (c *maildoso) Verify(ctx context.Context) error {
	_, err := c.lookup(ctx, url.Values{"limit": {"1"}, "short": {"true"}})
	return err
}

func (c *maildoso) List(ctx context.Context) ([]Mailbox, error) {
	var out []Mailbox
	for offset, page := 0, 0; page < maxPages; page++ {
		res, err := c.lookup(ctx, url.Values{
			"limit":    {strconv.Itoa(maildosoPageSize)},
			"offset":   {strconv.Itoa(offset)},
			"order_by": {"id"},
		})
		if err != nil {
			return nil, err
		}
		for _, a := range res.Items {
			m := a.mailbox()
			c.cache.put(m.ID, a.credentials())
			out = append(out, m)
			if len(out) >= MaxMailboxes {
				return out, nil
			}
		}
		offset += len(res.Items)
		if len(res.Items) == 0 || offset >= res.Meta.Total {
			break
		}
	}
	return out, nil
}

// Credentials comes from List's cache, else from a lookup by address.
func (c *maildoso) Credentials(ctx context.Context, m Mailbox) (Credentials, error) {
	if cr, ok := c.cache.get(m.ID); ok {
		return cr, nil
	}
	if m.Email == "" {
		return Credentials{}, vendorErr(VendorMaildoso, 0, "mailbox address is required", ErrNotFound)
	}
	res, err := c.lookup(ctx, url.Values{"keyword": {m.Email}, "limit": {"50"}})
	if err != nil {
		return Credentials{}, err
	}
	for _, a := range res.Items {
		if strings.EqualFold(a.Email, m.Email) {
			return a.credentials(), nil
		}
	}
	return Credentials{}, vendorErr(VendorMaildoso, http.StatusOK, "not found", ErrNotFound)
}

// DomainCapabilities: Maildoso's API sets a redirect but has no DNS record endpoint.
func (c *maildoso) DomainCapabilities() DomainCapabilities {
	return DomainCapabilities{Forwarding: true, ForwardingRemove: true}
}

// Domains reads GET /v1/user/domains, which returns every domain in one array.
func (c *maildoso) Domains(ctx context.Context) ([]Domain, error) {
	var res []struct {
		ID         int     `json:"id"`
		DomainName string  `json:"domain_name"`
		RedirectTo *string `json:"redirect_to"`
	}
	if err := c.t.do(ctx, call{method: http.MethodGet, path: "/v1/user/domains", limit: listBodyLimit}, &res); err != nil {
		return nil, err
	}
	out := make([]Domain, 0, min(len(res), MaxDomains))
	for _, d := range res {
		dom := Domain{ID: strconv.Itoa(d.ID), Name: d.DomainName}
		if d.RedirectTo != nil {
			dom.Forwarding = *d.RedirectTo
		}
		out = append(out, dom)
		if len(out) >= MaxDomains {
			break
		}
	}
	return out, nil
}

// SetForwarding calls PUT /v1/user/domains with redirect_to, or null to remove it.
func (c *maildoso) SetForwarding(ctx context.Context, d Domain, target string) error {
	target, err := forwardingTarget(VendorMaildoso, target, c.DomainCapabilities())
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(strings.TrimSpace(d.ID))
	if err != nil || id <= 0 {
		return invalid(VendorMaildoso, "domain id is required")
	}
	var redirect *string
	if target != "" {
		redirect = &target
	}
	body := []map[string]any{{"id": id, "redirect_to": redirect}}
	return c.t.do(ctx, call{method: http.MethodPut, path: "/v1/user/domains", body: body}, nil)
}

// UpsertDNSRecord is refused: Maildoso documents no DNS record endpoint.
func (c *maildoso) UpsertDNSRecord(context.Context, Domain, DNSRecord) error {
	return unsupported(VendorMaildoso, "DNS records are not supported")
}
