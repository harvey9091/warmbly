package jobs

import (
	"strings"
	"time"

	"github.com/mileusna/useragent"
	"github.com/warmbly/warmbly/internal/repository"
)

// isMachineOpen reports whether an open event came from an automated fetcher
// rather than a human-rendered view. The edge already filters crawlers and
// security scanners outright; this classifies the gray zone we still WANT to
// count (it is real delivery signal) but must not present as a human open:
//
//   - Apple Mail Privacy Protection prefetches every pixel at delivery time
//     with a WebKit UA that ends at the engine token. A real Safari/Mail
//     render continues with "Version/... Safari/...", so the bare suffix is
//     the canonical MPP fingerprint.
//   - A missing UA is never a real mail client or browser.
//
// Gmail's image proxy is deliberately treated as HUMAN: it fetches at open
// time (not delivery), and it is the only open signal Gmail exposes.
func isMachineOpen(userAgent *string) bool {
	if userAgent == nil {
		return true
	}
	ua := strings.ToLower(strings.TrimSpace(*userAgent))
	if ua == "" {
		return true
	}
	return strings.HasSuffix(ua, "(khtml, like gecko)")
}

// isInstant reports whether an engagement arrived so soon after the step was
// dispatched that no person could have read the email yet. Security gateways
// (Safe Links, Proofpoint, Mimecast) open the pixel and walk every link at
// delivery time with an ordinary browser UA, which is exactly what the UA
// rules cannot see.
//
// The anchor is dispatch to the worker, so `window` has to cover the SMTP
// handshake, the sending provider's queue and transit to the recipient before
// the arrival scan it is aimed at. It is operator-editable for that reason:
// how long that takes is a property of the deployment, not of the code.
//
// An unknown dispatch time never counts as instant, and neither does an event
// stamped before it: a clock skewed backwards must not mark everything human.
func isInstant(sentAt *time.Time, at time.Time, window time.Duration) bool {
	if sentAt == nil {
		return false
	}
	since := at.Sub(*sentAt)
	return since >= 0 && since < window
}

// isScannerSource reports whether the tracking edge recognised the request's
// source as a mail-filtering network. That verdict outranks the user agent:
// the whole point of the network rules is that a security gateway walks a
// message with an ordinary browser's user agent.
func isScannerSource(scanner *string) bool {
	return scanner != nil && strings.TrimSpace(*scanner) != ""
}

// classifyClick applies the per-event click rules (the burst rule needs the
// click log and lives in the consumer). It returns whether the click is
// automated and the reason recorded with it; an empty reason is a person.
func classifyClick(userAgent, scanner *string, sentAt *time.Time, at time.Time, window time.Duration) (bool, string) {
	if isScannerSource(scanner) {
		return true, repository.LinkClickReasonScanner
	}
	if userAgent == nil || strings.TrimSpace(*userAgent) == "" {
		return true, repository.LinkClickReasonPrefetch
	}
	if isInstant(sentAt, at, window) {
		return true, repository.LinkClickReasonInstant
	}
	return false, ""
}

// eventTime is when the tracking service saw the event, falling back to now
// when the stamp is missing or unreadable, so consumer lag never turns a
// delivery-time scan into a plausible human open.
func eventTime(stamp string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, stamp); err == nil {
		return t
	}
	return time.Now()
}

// classifyOpen applies the per-event open rules and names the one that
// caught it: scanner for a fetch from a known mail-filtering network,
// prefetch for a mail proxy or a fetch with no browser, instant for a fetch
// inside the machine window after dispatch. An empty reason is a person.
func classifyOpen(userAgent, scanner *string, sentAt *time.Time, at time.Time, window time.Duration) (bool, string) {
	if isScannerSource(scanner) {
		return true, repository.EmailOpenReasonScanner
	}
	if isMachineOpen(userAgent) {
		return true, repository.EmailOpenReasonPrefetch
	}
	if isInstant(sentAt, at, window) {
		return true, repository.EmailOpenReasonInstant
	}
	return false, ""
}

// clientName names the mail client or image proxy behind a user agent when
// it says so; empty for a plain browser, which the parsed fields describe.
func clientName(userAgent string) string {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	switch {
	case ua == "":
		return ""
	case strings.Contains(ua, "googleimageproxy"):
		return "Gmail"
	case strings.Contains(ua, "yahoomailproxy"), strings.Contains(ua, "yahoo mail"):
		return "Yahoo Mail"
	case strings.Contains(ua, "outlook"), strings.Contains(ua, "microsoft office"):
		return "Outlook"
	case strings.Contains(ua, "thunderbird"):
		return "Thunderbird"
	case strings.Contains(ua, "superhuman"):
		return "Superhuman"
	case strings.Contains(ua, "protonmail"), strings.Contains(ua, "proton mail"):
		return "Proton Mail"
	case strings.Contains(ua, "hey.com"):
		return "HEY"
	case strings.HasSuffix(ua, "(khtml, like gecko)"):
		// Apple Mail Privacy Protection's prefetch fingerprint.
		return "Apple Mail"
	}
	return ""
}

// deviceType folds the parser's flags into desktop, mobile, tablet or unknown.
func deviceType(ua useragent.UserAgent) string {
	switch {
	case ua.Tablet:
		return "tablet"
	case ua.Mobile:
		return "mobile"
	case ua.Desktop:
		return "desktop"
	default:
		return "unknown"
	}
}
