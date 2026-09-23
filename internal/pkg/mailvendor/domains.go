package mailvendor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

// DNS record types a DomainManager may write.
const (
	DNSTypeA     = "A"
	DNSTypeAAAA  = "AAAA"
	DNSTypeCNAME = "CNAME"
	DNSTypeTXT   = "TXT"
)

// MaxDomains caps Domains; a vendor account larger than this is truncated.
const MaxDomains = 10000

// ErrUnsupported is returned for an operation this vendor's API does not offer.
var ErrUnsupported = errors.New("mailvendor: not supported by this vendor")

// DomainManager is implemented by vendors whose API manages the domains they sell.
// Discover it with a type assertion on a Client.
type DomainManager interface {
	Domains(ctx context.Context) ([]Domain, error)
	// DomainCapabilities says what this vendor's API can do for a domain.
	DomainCapabilities() DomainCapabilities
	// SetForwarding makes the domain's root (and www where the vendor does both) redirect to url. Empty url removes it where supported.
	SetForwarding(ctx context.Context, d Domain, url string) error
	// UpsertDNSRecord creates or replaces one record. Type is "CNAME", "TXT", "A" or "AAAA"; Name is the full host name.
	// It replaces the records of the same type and name; for TXT only those sharing the value's
	// "key=" prefix (so an SPF record survives a verification token), or an identical value when there is no "=".
	UpsertDNSRecord(ctx context.Context, d Domain, r DNSRecord) error
}

// Every vendor's API sets a forward, so every client is a DomainManager.
var (
	_ DomainManager = (*inboxKit)(nil)
	_ DomainManager = (*zapmail)(nil)
	_ DomainManager = (*forge)(nil)
	_ DomainManager = (*maildoso)(nil)
	_ DomainManager = (*cheapInboxes)(nil)
	_ DomainManager = (*scaledMail)(nil)
)

// Domain is one domain the vendor account holds.
type Domain struct {
	ID   string
	Name string
	// Forwarding is the current forwarding target if the API reports it.
	Forwarding string
}

// DNSRecord is one record to write. TTL is seconds; 0 leaves the vendor's default.
type DNSRecord struct {
	Type  string
	Name  string
	Value string
	TTL   int
}

// DomainCapabilities says what a vendor's domain API supports.
type DomainCapabilities struct {
	Forwarding bool
	// ForwardingRemove is true when SetForwarding with an empty url removes the forwarding.
	ForwardingRemove bool
	// ForwardingReviewed is true when a forwarding change is a request the vendor's staff apply later.
	ForwardingReviewed bool
	DNS                bool
	DNSTypes           []string
}

const (
	maxTXTLength = 4096
	maxTTL       = 86400
)

func allDNSTypes() []string {
	return []string{DNSTypeA, DNSTypeAAAA, DNSTypeCNAME, DNSTypeTXT}
}

func invalid(vendor, reason string) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalidConfig, vendor, reason)
}

func unsupported(vendor, reason string) error {
	return vendorErr(vendor, 0, reason, ErrUnsupported)
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// normalizeHost lowercases a host name and drops a trailing dot.
func normalizeHost(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

// domainName validates and normalizes the name of the domain being changed.
func domainName(vendor string, d Domain) (string, error) {
	name := normalizeHost(d.Name)
	if name == "" || !strings.Contains(name, ".") || hasControl(name) || strings.ContainsAny(name, " /@") {
		return "", invalid(vendor, "a valid domain name is required")
	}
	return name, nil
}

// forwardingTarget validates a forwarding url; empty is returned as is and means remove.
func forwardingTarget(vendor, raw string, caps DomainCapabilities) (string, error) {
	raw = strings.TrimSpace(raw)
	if !caps.Forwarding {
		return "", unsupported(vendor, "domain forwarding is not supported")
	}
	if raw == "" {
		if !caps.ForwardingRemove {
			return "", unsupported(vendor, "removing domain forwarding is not supported")
		}
		return "", nil
	}
	if len(raw) > 2048 || hasControl(raw) {
		return "", invalid(vendor, "forwarding URL is not valid")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", invalid(vendor, "forwarding URL must be an http or https URL")
	}
	return raw, nil
}

// prepareRecord validates r against the domain and the vendor's capabilities and normalizes it.
func prepareRecord(vendor string, d Domain, r DNSRecord, caps DomainCapabilities) (DNSRecord, string, error) {
	domain, err := domainName(vendor, d)
	if err != nil {
		return DNSRecord{}, "", err
	}
	if !caps.DNS {
		return DNSRecord{}, "", unsupported(vendor, "DNS records are not supported")
	}
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	if !slices.Contains(caps.DNSTypes, r.Type) {
		return DNSRecord{}, "", unsupported(vendor, "DNS record type is not supported")
	}
	r.Name = normalizeHost(r.Name)
	if r.Name != domain && !strings.HasSuffix(r.Name, "."+domain) {
		return DNSRecord{}, "", invalid(vendor, "record name must be the domain or a host under it")
	}
	if hasControl(r.Name) || strings.ContainsAny(r.Name, " /@") {
		return DNSRecord{}, "", invalid(vendor, "record name is not valid")
	}
	if r.TTL < 0 || r.TTL > maxTTL {
		return DNSRecord{}, "", invalid(vendor, "record TTL is out of range")
	}
	r.Value = strings.TrimSpace(r.Value)
	if r.Value == "" || len(r.Value) > maxTXTLength || hasControl(r.Value) {
		return DNSRecord{}, "", invalid(vendor, "record value is not valid")
	}
	switch r.Type {
	case DNSTypeA:
		if ip := net.ParseIP(r.Value); ip == nil || ip.To4() == nil {
			return DNSRecord{}, "", invalid(vendor, "A record value must be an IPv4 address")
		}
	case DNSTypeAAAA:
		if ip := net.ParseIP(r.Value); ip == nil || ip.To4() != nil {
			return DNSRecord{}, "", invalid(vendor, "AAAA record value must be an IPv6 address")
		}
	case DNSTypeCNAME:
		if r.Name == domain {
			return DNSRecord{}, "", invalid(vendor, "a CNAME cannot sit at the domain root")
		}
		r.Value = normalizeHost(r.Value)
		if !strings.Contains(r.Value, ".") || strings.ContainsAny(r.Value, " /@:") {
			return DNSRecord{}, "", invalid(vendor, "CNAME value must be a host name")
		}
	}
	return r, domain, nil
}

// relativeHost turns a full host name into the label vendors expect, "@" for the root.
func relativeHost(fqdn, domain string) string {
	if fqdn == domain {
		return "@"
	}
	return strings.TrimSuffix(fqdn, "."+domain)
}

// absoluteHost reads a vendor's record name, relative or full, as a full host name.
func absoluteHost(name, domain string) string {
	h := normalizeHost(name)
	switch {
	case h == "" || h == "@":
		return domain
	case h == domain || strings.HasSuffix(h, "."+domain):
		return h
	}
	return h + "." + domain
}

// existingRecord is a record already on the domain, in the vendor-neutral shape the planner reads.
type existingRecord struct {
	ID    string
	Type  string
	Name  string // full host name
	Value string
	// Locked marks a record the vendor manages itself and will not let us change.
	Locked bool
}

// dnsPlan is what an upsert has to do with the records already on the domain.
type dnsPlan struct {
	noop bool
	// update is the index of the record to overwrite, or -1 to create one.
	update int
	remove []int
}

func unquoteTXT(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}

// txtKey is the part of a TXT value that identifies what it is for.
func txtKey(v string) string {
	v = unquoteTXT(v)
	if i := strings.IndexByte(v, '='); i > 0 {
		return strings.ToLower(v[:i])
	}
	return v
}

func sameValue(typ, a, b string) bool {
	switch typ {
	case DNSTypeTXT:
		return unquoteTXT(a) == unquoteTXT(b)
	case DNSTypeA, DNSTypeAAAA:
		ia, ib := net.ParseIP(strings.TrimSpace(a)), net.ParseIP(strings.TrimSpace(b))
		return ia != nil && ia.Equal(ib)
	}
	return normalizeHost(a) == normalizeHost(b)
}

// planUpsert finds the records r replaces: same type and name, and for TXT the same key.
func planUpsert(existing []existingRecord, r DNSRecord) dnsPlan {
	var matches []int
	for i, e := range existing {
		if !strings.EqualFold(e.Type, r.Type) || normalizeHost(e.Name) != r.Name {
			continue
		}
		if r.Type == DNSTypeTXT && txtKey(e.Value) != txtKey(r.Value) {
			continue
		}
		matches = append(matches, i)
	}
	if len(matches) == 0 {
		return dnsPlan{update: -1}
	}
	if len(matches) == 1 && sameValue(r.Type, existing[matches[0]].Value, r.Value) {
		return dnsPlan{noop: true, update: -1}
	}
	p := dnsPlan{update: matches[0]}
	if len(matches) > 1 {
		p.remove = matches[1:]
	}
	return p
}

// checkPlan refuses a plan that would change a record the vendor manages itself.
func checkPlan(vendor string, existing []existingRecord, p dnsPlan) error {
	idx := p.remove
	if p.update >= 0 {
		idx = append([]int{p.update}, idx...)
	}
	for _, i := range idx {
		if existing[i].Locked {
			return vendorErr(vendor, 0, "the existing record is managed by the vendor", ErrUnsupported)
		}
	}
	return nil
}

// needIDs refuses a plan that touches a record the vendor listed without an id.
func needIDs(vendor string, existing []existingRecord, p dnsPlan) error {
	idx := p.remove
	if p.update >= 0 {
		idx = append([]int{p.update}, idx...)
	}
	for _, i := range idx {
		if existing[i].ID == "" {
			return vendorErr(vendor, 0, "the vendor listed the existing record without an id", ErrUnsupported)
		}
	}
	return nil
}

// ttlOrNil returns nil for the vendor's default TTL, so it is omitted from the body.
func ttlOrNil(ttl int) *int {
	if ttl <= 0 {
		return nil
	}
	return &ttl
}
