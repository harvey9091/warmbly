package tasks

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// UTMParams are the campaign-level UTM values stamped on every link.
// utm_content is not here because it is per link: the link's own text.
type UTMParams struct {
	Source   string
	Medium   string
	Campaign string
}

// LinkTracking describes what the send path does to the links of one email:
// rewrite them to click tickets, tag them with UTM parameters, or both.
type LinkTracking struct {
	TaskID     uuid.UUID
	CampaignID uuid.UUID
	// TrackingDomain hosts the click tickets. Empty means no ticket can be
	// resolved, so links are never wrapped even when Wrap is set.
	TrackingDomain string
	Wrap           bool
	// UTM, when set, tags every link that does not already carry the
	// parameter. Hand-written UTM values always win.
	UTM *UTMParams
}

// Defaults for automatic UTM tagging when the campaign leaves a value empty.
const (
	utmDefaultSource = "warmbly"
	utmDefaultMedium = "email"
	utmMaxLabelRunes = 120
	utmMaxSlugRunes  = 64
)

// CampaignUTM resolves the campaign's UTM settings to the values the send
// path stamps, filling defaults for anything left empty. nil when the
// campaign does not tag links.
func CampaignUTM(c *models.Campaign) *UTMParams {
	if c == nil || !c.UTMTracking {
		return nil
	}
	p := &UTMParams{
		Source:   strings.TrimSpace(c.UTMSource),
		Medium:   strings.TrimSpace(c.UTMMedium),
		Campaign: strings.TrimSpace(c.UTMCampaign),
	}
	if p.Source == "" {
		p.Source = utmDefaultSource
	}
	if p.Medium == "" {
		p.Medium = utmDefaultMedium
	}
	if p.Campaign == "" {
		p.Campaign = utmSlug(c.Name)
	}
	if p.Campaign == "" {
		p.Campaign = "campaign"
	}
	return p
}

// anchorTag matches one <a ...href=...>...</a>, capturing the href value
// (double-quoted, single-quoted or bare) and the inner HTML the label is
// read from. The attribute must follow whitespace so data-href and the like
// never pass for it. Only anchors are touched: a <link href> in the head is
// a stylesheet, and redirecting it through a click ticket breaks it.
var anchorTag = regexp.MustCompile(`(?is)<a\b([^>]*?)\shref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))([^>]*)>(.*?)</a>`)

var (
	htmlTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	imgAlt     = regexp.MustCompile(`(?is)<img\b[^>]*\balt\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	whitespace = regexp.MustCompile(`\s+`)
)

// TrackLinks rewrites the anchors of an HTML body per opts and returns the
// click tickets it minted (nil when nothing was wrapped). A wrapped link
// carries only the opaque ticket (https://<domain>/c/<id>); the destination,
// UTM parameters included, lives in the returned row. The caller MUST persist
// the rows before using the rewritten body, and fall back on failure, so an
// email can never ship dead tickets.
//
// Skipped, and left exactly as written: anchors, mailto: and tel: links,
// data: and javascript: URLs, anything that is not http(s), and links that
// already point at the tracking host.
func TrackLinks(htmlBody string, opts LinkTracking) (string, []repository.TrackedLink) {
	trackingDomain := config.NormalizeTrackingHost(opts.TrackingDomain)
	wrap := opts.Wrap && trackingDomain != ""
	if !wrap && opts.UTM == nil {
		return htmlBody, nil
	}

	var links []repository.TrackedLink
	position := 0

	result := anchorTag.ReplaceAllStringFunc(htmlBody, func(match string) string {
		m := anchorTag.FindStringSubmatch(match)
		before, quoted, single, bare, after, inner := m[1], m[2], m[3], m[4], m[5], m[6]
		rawHref := quoted
		if rawHref == "" {
			rawHref = single
		}
		if rawHref == "" {
			rawHref = bare
		}
		dest := strings.TrimSpace(html.UnescapeString(rawHref))
		if !trackableURL(dest, trackingDomain) {
			return match
		}
		position++
		label := linkLabel(inner)

		if opts.UTM != nil {
			dest = withUTM(dest, opts.UTM, utmContent(label, position))
		}

		href := html.EscapeString(dest)
		if wrap {
			id := uuid.New()
			links = append(links, repository.TrackedLink{
				ID:          id,
				TaskID:      opts.TaskID,
				CampaignID:  opts.CampaignID,
				Destination: dest,
				Label:       label,
			})
			href = config.TrackingURL(trackingDomain, "/c/"+id.String())
		}
		return `<a` + before + ` href="` + href + `"` + after + `>` + inner + `</a>`
	})

	return result, links
}

// WrapLinksForTracking rewrites every external link to a click ticket and
// returns the minted rows. Kept as the wrap-only form of TrackLinks.
func WrapLinksForTracking(htmlBody string, taskID, campaignID uuid.UUID, trackingDomain string) (string, []repository.TrackedLink) {
	return TrackLinks(htmlBody, LinkTracking{
		TaskID:         taskID,
		CampaignID:     campaignID,
		TrackingDomain: trackingDomain,
		Wrap:           true,
	})
}

// trackableURL reports whether a destination is one a click ticket can
// redirect to and a UTM tag makes sense on. A link already pointing at the
// tracking host (its host, not merely a URL mentioning it) is left alone.
func trackableURL(dest, trackingDomain string) bool {
	lower := strings.ToLower(dest)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	// An unsubscribe link is never a click to count or a redirect to bounce
	// through, and a UTM tag on it would only clutter the opt-out page URL.
	if strings.Contains(lower, unsubscribePathMarker) {
		return false
	}
	if trackingDomain == "" {
		return true
	}
	// A destination the URL parser rejects cannot be redirected to, so it
	// is left as written rather than turned into a dead ticket.
	u, err := url.Parse(dest)
	if err != nil || u.Hostname() == "" {
		return false
	}
	return config.NormalizeTrackingHost(u.Host) != trackingDomain
}

// bareURL matches an http(s) URL in plain text, stopping at whitespace and
// the characters that close it in prose.
var bareURL = regexp.MustCompile(`(?i)https?://[^\s<>"'` + "`" + `]+`)

// TagPlainTextLinks appends UTM parameters to every bare URL of a plain-text
// body. There is no anchor text, so utm_content numbers the links in order.
// Trailing punctuation that belongs to the sentence stays outside the URL.
func TagPlainTextLinks(body string, utm *UTMParams, trackingDomain string) string {
	if utm == nil || body == "" {
		return body
	}
	trackingDomain = config.NormalizeTrackingHost(trackingDomain)
	position := 0
	return bareURL.ReplaceAllStringFunc(body, func(match string) string {
		dest := strings.TrimRight(match, ".,;:!?)]}")
		trail := match[len(dest):]
		if !trackableURL(dest, trackingDomain) {
			return match
		}
		position++
		return withUTM(dest, utm, utmContent("", position)) + trail
	})
}

// linkLabel is the anchor's visible text: tags stripped, entities decoded,
// whitespace collapsed. An image link falls back to the image's alt text.
func linkLabel(inner string) string {
	text := html.UnescapeString(htmlTag.ReplaceAllString(inner, " "))
	text = strings.TrimSpace(whitespace.ReplaceAllString(text, " "))
	if text == "" {
		if m := imgAlt.FindStringSubmatch(inner); m != nil {
			alt := m[1]
			if alt == "" {
				alt = m[2]
			}
			text = strings.TrimSpace(whitespace.ReplaceAllString(html.UnescapeString(alt), " "))
		}
	}
	return truncateRunes(text, utmMaxLabelRunes)
}

// utmContent names the link inside the email: its text as a slug, or its
// ordinal when it has none (a bare image or icon).
func utmContent(label string, position int) string {
	if s := utmSlug(label); s != "" {
		return s
	}
	return fmt.Sprintf("link_%d", position)
}

// utmSlug lowercases a label and joins its words with underscores
// ("See our Pricing!" -> "see_our_pricing"), the shape analytics tools
// expect in a utm value.
func utmSlug(s string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			b.WriteRune(r)
			continue
		}
		pendingSep = true
	}
	return truncateRunes(b.String(), utmMaxSlugRunes)
}

// withUTM appends the UTM parameters the destination does not already carry,
// keeping the existing query and fragment exactly as written. The URL is
// never re-encoded: a customer's signed or oddly-encoded link stays intact.
func withUTM(dest string, p *UTMParams, content string) string {
	u, err := url.Parse(dest)
	if err != nil {
		return dest
	}
	existing := u.Query()
	pairs := [][2]string{
		{"utm_source", p.Source},
		{"utm_medium", p.Medium},
		{"utm_campaign", p.Campaign},
		{"utm_content", content},
	}
	var add []string
	for _, kv := range pairs {
		if kv[1] == "" || existing.Has(kv[0]) {
			continue
		}
		add = append(add, kv[0]+"="+url.QueryEscape(kv[1]))
	}
	if len(add) == 0 {
		return dest
	}

	base, fragment, hasFragment := strings.Cut(dest, "#")
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
		if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
			sep = ""
		}
	}
	out := base + sep + strings.Join(add, "&")
	if hasFragment {
		out += "#" + fragment
	}
	return out
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n]), "_ ")
}
