package mailvendor

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// ScaledMail: https://api.scaledmail.com/llms.txt
type scaledMail struct {
	t     *transport
	org   string
	cache credCache
}

// scaledMailPerSecond is the documented limit; going over it earns a temporary block.
const scaledMailPerSecond = 5

func newScaledMail(vals map[string]string, o options) *scaledMail {
	t := newTransport(VendorScaledMail, "https://server.scaledmail.com/api/v1", o, bearer(vals[FieldAPIKey]), scaledMailPerSecond)
	return &scaledMail{t: t, org: vals[FieldOrganizationID]}
}

func (c *scaledMail) Vendor() string { return VendorScaledMail }

func (c *scaledMail) Verify(ctx context.Context) error {
	return c.t.do(ctx, call{method: http.MethodGet, path: "/organizations"}, nil)
}

type scaledMailDomain struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Redirect string `json:"redirect"`
}

type scaledMailMailbox struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Alias     string `json:"alias"`
	Status    string `json:"status"`
	Email     string `json:"email"`
	OrderType string `json:"order_type"`
	Password  string `json:"mailbox_password"`
}

func (c *scaledMail) domains(ctx context.Context) ([]scaledMailDomain, error) {
	var res struct {
		Domains []scaledMailDomain `json:"domains"`
	}
	q := url.Values{"organization_id": {c.org}}
	if err := c.t.do(ctx, call{method: http.MethodGet, path: "/domains", query: q}, &res); err != nil {
		return nil, err
	}
	return res.Domains, nil
}

func (c *scaledMail) mailboxes(ctx context.Context, domainID string) ([]scaledMailMailbox, error) {
	var res struct {
		// Null when the domain belongs to another organization.
		Mailboxes []scaledMailMailbox `json:"mailboxes"`
	}
	q := url.Values{"organization_id": {c.org}, "password": {"true"}}
	if err := c.t.do(ctx, call{method: http.MethodGet, path: "/mailboxes/" + url.PathEscape(domainID), query: q}, &res); err != nil {
		return nil, err
	}
	return res.Mailboxes, nil
}

func (m scaledMailMailbox) email(domain string) string {
	if m.Email != "" {
		return m.Email
	}
	if m.Alias != "" && domain != "" {
		return m.Alias + "@" + domain
	}
	return ""
}

// List reads the domains, then each domain's mailboxes with their passwords.
func (c *scaledMail) List(ctx context.Context) ([]Mailbox, error) {
	domains, err := c.domains(ctx)
	if err != nil {
		return nil, err
	}
	var out []Mailbox
	for _, d := range domains {
		if d.ID == "" {
			continue
		}
		mbs, err := c.mailboxes(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range mbs {
			email := m.email(d.Domain)
			if email == "" {
				continue
			}
			// ScaledMail has no mailbox id; the address is the stable key.
			id := strings.ToLower(email)
			c.cache.put(id, Credentials{Password: m.Password})
			out = append(out, Mailbox{
				ID:        id,
				Email:     email,
				FirstName: m.FirstName,
				LastName:  m.LastName,
				Domain:    firstNonEmpty(d.Domain, domainOf(email)),
				Provider:  normalizeProvider(m.OrderType),
				Status:    m.Status,
			})
			if len(out) >= MaxMailboxes {
				return out, nil
			}
		}
	}
	return out, nil
}

// Credentials comes from List's cache, else re-reads the mailbox's domain.
func (c *scaledMail) Credentials(ctx context.Context, m Mailbox) (Credentials, error) {
	id := strings.ToLower(firstNonEmpty(m.ID, m.Email))
	if cr, ok := c.cache.get(id); ok {
		return cr, nil
	}
	domain := strings.ToLower(firstNonEmpty(m.Domain, domainOf(id)))
	domains, err := c.domains(ctx)
	if err != nil {
		return Credentials{}, err
	}
	for _, d := range domains {
		if !strings.EqualFold(d.Domain, domain) || d.ID == "" {
			continue
		}
		mbs, err := c.mailboxes(ctx, d.ID)
		if err != nil {
			return Credentials{}, err
		}
		for _, mb := range mbs {
			if strings.EqualFold(mb.email(d.Domain), id) {
				return Credentials{Password: mb.Password}, nil
			}
		}
	}
	return Credentials{}, vendorErr(VendorScaledMail, http.StatusOK, "not found", ErrNotFound)
}

// DomainCapabilities: ScaledMail's redirect change is a request its team applies; there is no DNS endpoint.
func (c *scaledMail) DomainCapabilities() DomainCapabilities {
	return DomainCapabilities{Forwarding: true, ForwardingRemove: true, ForwardingReviewed: true}
}

// Domains reads GET /domains, which lists the organization's active domains in one call.
func (c *scaledMail) Domains(ctx context.Context) ([]Domain, error) {
	domains, err := c.domains(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Domain, 0, min(len(domains), MaxDomains))
	for _, d := range domains {
		out = append(out, Domain{ID: d.ID, Name: d.Domain, Forwarding: d.Redirect})
		if len(out) >= MaxDomains {
			break
		}
	}
	return out, nil
}

// withScheme mirrors ScaledMail adding https:// to a redirect given without one.
func withScheme(u string) string {
	u = strings.TrimSpace(u)
	if u != "" && !strings.Contains(u, "://") {
		return "https://" + u
	}
	return u
}

// SetForwarding calls POST /swap-redirect/{domain}, which files a request ScaledMail's team applies later; it returns once accepted.
func (c *scaledMail) SetForwarding(ctx context.Context, d Domain, target string) error {
	target, err := forwardingTarget(VendorScaledMail, target, c.DomainCapabilities())
	if err != nil {
		return err
	}
	name, err := domainName(VendorScaledMail, d)
	if err != nil {
		return err
	}
	// The vendor refuses a request for the redirect a domain already has.
	if strings.EqualFold(strings.TrimRight(withScheme(d.Forwarding), "/"), strings.TrimRight(withScheme(target), "/")) {
		return nil
	}
	q := url.Values{"organization_id": {c.org}}
	body := map[string]string{"new_redirect": target}
	return c.t.do(ctx, call{method: http.MethodPost, path: "/swap-redirect/" + url.PathEscape(name), query: q, body: body}, nil)
}

// UpsertDNSRecord is refused: ScaledMail documents no DNS record endpoint.
func (c *scaledMail) UpsertDNSRecord(context.Context, Domain, DNSRecord) error {
	return unsupported(VendorScaledMail, "DNS records are not supported")
}
