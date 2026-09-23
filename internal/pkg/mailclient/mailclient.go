// Package mailclient reads the mail client, app or webmail, and device behind
// a tracking request, and never claims what a proxy's user agent cannot say.
package mailclient

import (
	"strings"

	"github.com/mileusna/useragent"
)

// ClientType values, matching models.EngagementClientApp and
// models.EngagementClientWebmail.
const (
	App     = "app"
	Webmail = "webmail"
)

// Locate is how far the request's network places the reader.
type Locate int

const (
	LocateFull   Locate = iota // the reader's own network
	LocateRegion               // a relay that keeps the region, not the city (Apple MPP)
	LocateNone                 // a provider's data centre (Gmail, Yahoo)
)

// Reading is what one request says about who made it; empty when unknown.
type Reading struct {
	Client     string // product name: "Gmail", "Apple Mail", "Outlook"
	ClientType string // App, Webmail or empty
	// DeviceHidden marks a fetch by a provider's image proxy.
	DeviceHidden   bool
	DeviceType     string // "desktop", "mobile", "tablet" or empty
	OS             string
	Browser        string
	BrowserVersion string
	Locate         Locate
}

// Detect reads a user agent. click marks a followed link rather than a pixel:
// its user agent is the browser the link opened in, so it never names webmail.
func Detect(userAgent string, click bool) Reading {
	raw := strings.TrimSpace(userAgent)
	if raw == "" {
		return Reading{}
	}
	ua := strings.ToLower(raw)

	// Proxies first: their user agents carry a fixed, fake browser.
	for _, p := range proxies {
		if p.match(ua) {
			return Reading{Client: p.client, DeviceHidden: true, Locate: p.locate}
		}
	}

	parsed := useragent.Parse(raw)
	r := Reading{
		OS:             osName(parsed),
		Browser:        parsed.Name,
		BrowserVersion: parsed.Version,
		DeviceType:     deviceType(parsed),
	}

	// A named client on a click made the request itself (an in-app browser,
	// Outlook's link check), so the click is still in that app.
	if a, ok := namedApp(ua); ok {
		r.Client, r.ClientType = a.client, App
		r.Browser, r.BrowserVersion = "", ""
		if r.OS == "" {
			r.OS = a.os
		}
		if r.DeviceType == "" {
			r.DeviceType = a.device
		}
		return r
	}

	if IsBareWebKit(ua) {
		// Mac Mail, iPad Mail or the new Outlook for Windows: an app, on a
		// device the string cannot name. The parser reads its platform as a
		// browser, so nothing else is kept.
		if click {
			return Reading{}
		}
		return Reading{Client: BareWebKitClient, ClientType: App}
	}

	if click {
		return r
	}

	switch {
	case isAppleMailWebView(ua, parsed):
		// iOS Mail renders in a WKWebView, which drops Safari's own tokens.
		r.Client, r.ClientType = "Apple Mail", App
		r.Browser, r.BrowserVersion = "", ""
	case isAndroidWebView(ua):
		// A mail app's embedded WebView; the app itself goes unnamed.
		r.ClientType = App
		r.Browser, r.BrowserVersion = "", ""
	case isBrowser(ua, parsed):
		r.ClientType = Webmail
	}
	return r
}

// proxy is a provider that fetches every image itself.
type proxy struct {
	client string
	locate Locate
	match  func(ua string) bool
}

var proxies = []proxy{
	{client: "Gmail", locate: LocateNone, match: func(ua string) bool {
		return strings.Contains(ua, "googleimageproxy") || strings.Contains(ua, "via ggpht.com")
	}},
	{client: "Yahoo Mail", locate: LocateNone, match: func(ua string) bool {
		return strings.Contains(ua, "yahoomailproxy")
	}},
	// Mail Privacy Protection sends nothing but the product token.
	{client: "Apple Mail", locate: LocateRegion, match: func(ua string) bool {
		return ua == "mozilla/5.0"
	}},
	{client: "HEY", locate: LocateNone, match: func(ua string) bool {
		return strings.Contains(ua, "hey.com/imageproxy")
	}},
	{client: "Fastmail", locate: LocateNone, match: func(ua string) bool {
		return strings.Contains(ua, "fastmailua")
	}},
	{client: "Seznam Email", locate: LocateNone, match: func(ua string) bool {
		return strings.Contains(ua, "seznamemailproxy")
	}},
}

// BareWebKitClient names the stripped WebKit signature Mac Mail and the new
// Outlook for Windows both send, from the reader's own network (issue #564).
const BareWebKitClient = "Apple Mail or Outlook"

// IsBareWebKit reports the stripped WebKit user agent with nothing after the
// engine (Mail on a Mac, the new Outlook for Windows).
func IsBareWebKit(ua string) bool {
	ua = strings.ToLower(strings.TrimSpace(ua))
	return strings.Contains(ua, "applewebkit/") && strings.HasSuffix(ua, "(khtml, like gecko)")
}

// IsPrivacyProxy reports Mail Privacy Protection's bare product token.
func IsPrivacyProxy(ua string) bool {
	return strings.ToLower(strings.TrimSpace(ua)) == "mozilla/5.0"
}

// app is a mail client that names itself in its user agent.
type app struct {
	client string
	tokens []string
	// os and device are what the client implies when the string names no
	// platform (Outlook's Word renderer runs only on Windows).
	os, device string
}

// apps are matched in order, so a more specific token sits above a looser one.
var apps = []app{
	{client: "Outlook", tokens: []string{"outlook-ios-android", "outlook-android"}, os: "Android", device: "mobile"},
	{client: "Outlook", tokens: []string{"outlook-ios"}, os: "iOS", device: "mobile"},
	{client: "Outlook", tokens: []string{"macoutlook"}, os: "macOS", device: "desktop"},
	{client: "Outlook", tokens: []string{"microsoft outlook", "ms-office", "msoffice"}, os: "Windows", device: "desktop"},
	{client: "Thunderbird", tokens: []string{"thunderbird/"}, device: "desktop"},
	{client: "eM Client", tokens: []string{"em client"}, device: "desktop"},
	{client: "Mailbird", tokens: []string{"mailbird/"}, os: "Windows", device: "desktop"},
	{client: "Mailspring", tokens: []string{"mailspring/"}, device: "desktop"},
	{client: "BlueMail", tokens: []string{"bluemail/"}},
	{client: "Superhuman", tokens: []string{"superhuman"}},
}

func namedApp(ua string) (app, bool) {
	for _, a := range apps {
		for _, t := range a.tokens {
			if strings.Contains(ua, t) {
				return a, true
			}
		}
	}
	return app{}, false
}

// inAppTokens mark an iOS WebView that belongs to a browser or another app.
var inAppTokens = []string{
	"safari/", "crios", "fxios", "edgios", "opios", "gsa/", "fban", "fbav", "instagram",
	"line/", "micromessenger", "twitter", "linkedinapp", "snapchat", "pinterest", "duckduckgo",
}

// isAppleMailWebView is Mail on an iPhone or iPad: iOS WebKit with the build
// token and nothing a browser or another app adds.
func isAppleMailWebView(ua string, parsed useragent.UserAgent) bool {
	if parsed.OS != useragent.IOS || !strings.Contains(ua, "applewebkit/") || !strings.Contains(ua, "mobile/") {
		return false
	}
	for _, t := range inAppTokens {
		if strings.Contains(ua, t) {
			return false
		}
	}
	return true
}

// isAndroidWebView is an Android app's embedded WebView, which names no app.
func isAndroidWebView(ua string) bool {
	return strings.Contains(ua, "android") && strings.Contains(ua, "; wv)")
}

// isBrowser is a full browser: webmail read in a tab.
func isBrowser(ua string, parsed useragent.UserAgent) bool {
	if parsed.Bot {
		return false
	}
	switch parsed.Name {
	case useragent.Chrome, useragent.Firefox, useragent.Safari, useragent.Edge, useragent.Opera,
		useragent.Vivaldi, useragent.SamsungBrowser, useragent.MobileSafari:
		return strings.Contains(ua, "mozilla/5.0")
	}
	return false
}

// osName is the operating system as the dashboard names it.
func osName(ua useragent.UserAgent) string {
	switch ua.OS {
	case useragent.CrOS:
		return useragent.ChromeOS
	case useragent.WindowsNT, useragent.WindowsPhoneOS:
		return useragent.Windows
	}
	return ua.OS
}

// deviceType folds the parser's flags into desktop, mobile or tablet.
func deviceType(ua useragent.UserAgent) string {
	switch {
	case ua.Tablet:
		return "tablet"
	case ua.Mobile:
		return "mobile"
	case ua.Desktop:
		return "desktop"
	}
	return ""
}
