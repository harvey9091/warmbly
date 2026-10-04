package mailhost

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"

	"github.com/warmbly/warmbly/internal/pkg/safehttp"
)

const (
	// detectBudget bounds one domain's whole chain; DetectMany applies it per domain.
	detectBudget  = 12 * time.Second
	mxTimeout     = 4 * time.Second
	srvTimeout    = 3 * time.Second
	fetchTimeout  = 4 * time.Second
	maxBody       = 64 << 10
	manyWorkers   = 32
	defaultISPDB  = "https://autoconfig.thunderbird.net/v1.1/"
	maxDomainSize = 253
)

// Resolver is the DNS surface Detect needs; *net.Resolver satisfies it.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// Cache stores detections by normalized domain.
type Cache interface {
	Get(ctx context.Context, domain string) (Detection, bool)
	Set(ctx context.Context, domain string, d Detection)
}

// Detector runs the detection chain. The zero value is not usable; use NewDetector.
type Detector struct {
	resolver Resolver
	client   *http.Client
	cache    Cache
	// ispdbBase is the Thunderbird ISPDB prefix; the domain is appended.
	ispdbBase string
	// autoconfigURLs builds the provider-hosted autoconfig URLs for a domain.
	autoconfigURLs func(domain string) []string
}

// NewDetector builds a Detector. A nil resolver uses net.DefaultResolver, a nil
// client an SSRF-hardened one (domains are user input), a nil cache none.
func NewDetector(r Resolver, httpClient *http.Client, c Cache) *Detector {
	if r == nil {
		r = net.DefaultResolver
	}
	if httpClient == nil {
		httpClient = safehttp.Client(fetchTimeout)
	}
	return &Detector{
		resolver:       r,
		client:         httpClient,
		cache:          c,
		ispdbBase:      defaultISPDB,
		autoconfigURLs: defaultAutoconfigURLs,
	}
}

// WithoutISPDB stops the detector asking Thunderbird's ISPDB, a third party;
// the domain's own DNS and autoconfig files are still read.
func (d *Detector) WithoutISPDB() *Detector {
	d.ispdbBase = ""
	return d
}

func defaultAutoconfigURLs(domain string) []string {
	return []string{
		"https://autoconfig." + domain + "/mail/config-v1.1.xml?emailaddress=info@" + domain,
		"https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml",
	}
}

// NormalizeDomain lower-cases and trims a domain (or the domain of an address)
// and converts it to ASCII. It returns "" when the result is not a hostname.
func NormalizeDomain(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.ToLower(s), ".")
	if s == "" {
		return ""
	}
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return ""
	}
	if !validHostname(ascii) || !strings.Contains(ascii, ".") {
		return ""
	}
	return ascii
}

// validHostname accepts LDH labels only, so a value can go into a URL or a DNS query.
func validHostname(s string) bool {
	if s == "" || len(s) > maxDomainSize {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

// Detect works out where domain's mail is hosted. It never fails: what cannot
// be determined comes back with Source "none".
func (d *Detector) Detect(ctx context.Context, domain string) Detection {
	norm := NormalizeDomain(domain)
	if norm == "" {
		return Detection{Domain: strings.ToLower(strings.TrimSpace(domain)), Source: SourceNone, PasswordAuth: Unsupported}
	}
	if d.cache != nil {
		if hit, ok := d.cache.Get(ctx, norm); ok {
			return hit
		}
	}
	ctx, cancel := context.WithTimeout(ctx, detectBudget)
	defer cancel()
	det, cacheable := d.detect(ctx, norm)
	if cacheable && d.cache != nil {
		d.cache.Set(ctx, norm, det)
	}
	return det
}

// detect runs the chain; cacheable is false when a lookup failed transiently.
func (d *Detector) detect(ctx context.Context, domain string) (Detection, bool) {
	if h, v, ok := known(domain); ok {
		return detection(domain, h, v, SourceKnown, nil), true
	}

	mx, mxErr := d.lookupMX(ctx, domain)
	transient := mxErr != nil && !notFound(mxErr)
	for _, host := range mx {
		if h, v := classify(host); h != Unknown {
			return detection(domain, Refine(h, domain), v, SourceMX, mx), true
		}
	}

	if s := d.autoconfig(ctx, domain); s != nil {
		return fromSettings(domain, SourceAutoconfig, s, mx), true
	}
	if s := d.ispdb(ctx, domain, domain); s != nil {
		return fromSettings(domain, SourceISPDB, s, mx), true
	}
	if len(mx) > 0 {
		if reg, _ := registrableDomain(mx[0]); reg != "" && reg != domain {
			if s := d.ispdb(ctx, reg, domain); s != nil {
				return fromSettings(domain, SourceISPDB, s, mx), true
			}
		}
	}
	if s, err := d.srv(ctx, domain); s != nil {
		return fromSettings(domain, SourceSRV, s, mx), true
	} else if err != nil {
		transient = true
	}

	det := Detection{Domain: domain, Host: Unknown, Source: SourceNone, PasswordAuth: Unsupported, MX: mx}
	if len(mx) > 0 {
		det.Host = Other
		det.PasswordAuth = Password
	}
	// A deadline that ran out says nothing about the domain.
	if ctx.Err() != nil {
		transient = true
	}
	return det, !transient
}

// fromSettings names the provider behind discovered servers.
func fromSettings(domain, source string, s *Settings, mx []string) Detection {
	h, v := classify(s.SMTP.Host)
	if h == Unknown {
		h, v = classify(s.IMAP.Host)
	}
	if h == Unknown {
		return Detection{Domain: domain, Host: Other, Source: source, Settings: s, PasswordAuth: Password, MX: mx}
	}
	det := detection(domain, Refine(h, domain), v, source, mx)
	det.Settings = s
	return det
}

func (d *Detector) lookupMX(ctx context.Context, domain string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, mxTimeout)
	defer cancel()
	recs, err := d.resolver.LookupMX(ctx, domain)
	// The Go resolver can return usable records alongside an error for bad ones.
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Pref < recs[j].Pref })
	out := make([]string, 0, len(recs))
	seen := map[string]bool{}
	for _, r := range recs {
		if r == nil {
			continue
		}
		h := normHost(r.Host)
		// A null MX (RFC 7505) is a domain that receives no mail.
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	if len(out) > 0 {
		return out, nil
	}
	return nil, err
}

func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

// registrableDomain is host's eTLD+1.
func registrableDomain(host string) (string, bool) {
	label, suffix := registrable(host)
	if label == "" {
		return "", false
	}
	return label + "." + suffix, true
}

// DetectMany detects every distinct domain with bounded concurrency. The map
// is keyed by each input string as given; empty inputs are skipped.
func (d *Detector) DetectMany(ctx context.Context, domains []string) map[string]Detection {
	byNorm := map[string][]string{}
	var order []string
	for _, in := range domains {
		if strings.TrimSpace(in) == "" {
			continue
		}
		n := NormalizeDomain(in)
		if n == "" {
			n = "\x00" + in
		}
		if _, ok := byNorm[n]; !ok {
			order = append(order, n)
		}
		byNorm[n] = append(byNorm[n], in)
	}

	results := make([]Detection, len(order))
	sem := make(chan struct{}, manyWorkers)
	var wg sync.WaitGroup
	for i, n := range order {
		wg.Add(1)
		go func(i int, in string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = d.Detect(ctx, in)
				return
			}
			defer func() { <-sem }()
			results[i] = d.Detect(ctx, in)
		}(i, byNorm[n][0])
	}
	wg.Wait()

	out := make(map[string]Detection, len(domains))
	for i, n := range order {
		for _, in := range byNorm[n] {
			out[in] = results[i]
		}
	}
	return out
}
