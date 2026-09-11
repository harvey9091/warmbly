package mailhtml

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Finding is one thing mail clients will do to a body that its author did not
// ask for. Code is stable and machine-readable; Message is what the editor
// shows.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // error | warning | info
	Message  string `json:"message"`
}

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Gmail truncates a message past this many bytes and hides the rest behind a
// "View entire message" link, which also hides the unsubscribe footer and
// stops the open pixel firing for anyone who does not expand it.
const gmailClipBytes = 102 * 1024

// CSS the Word rendering engine behind Outlook 2016-2021 on Windows ignores
// outright. Every one of these silently collapses a layout there.
var wordUnsupportedProps = map[string]bool{
	"position": true, "float": true, "transform": true, "animation": true,
	"transition": true, "box-shadow": true, "text-shadow": true,
	"background-size": true, "object-fit": true, "flex": true, "gap": true,
}

// Elements no mail client renders. Each is also a spam signal in its own
// right, which matters more for cold outreach than for marketing mail.
var strippedElements = map[atom.Atom]string{
	atom.Script: "<script> is removed by every mail client and is a strong spam signal.",
	atom.Iframe: "<iframe> is removed by every mail client and is a strong spam signal.",
	atom.Form:   "<form> is removed or disabled by Gmail, Outlook and Apple Mail. Link to a hosted form instead.",
	atom.Object: "<object> is removed by every mail client.",
	atom.Embed:  "<embed> is removed by every mail client.",
	atom.Video:  "<video> plays only in Apple Mail. Use a poster image linking to the video.",
	atom.Audio:  "<audio> plays only in Apple Mail.",
}

// Lint reports what a body will run into on its way to an inbox.
//
// It is advisory and never blocks a send: an author who knows their audience
// is on Apple Mail is entitled to ignore every word of it. wireBytes is the
// size of the message as it will actually ship, after CSS inlining and the
// signature and footer are added, since that is what Gmail measures.
func Lint(bodyHTML string, wireBytes int) []Finding {
	if strings.TrimSpace(bodyHTML) == "" {
		return nil
	}
	var out []Finding
	add := func(code, sev, msg string) {
		out = append(out, Finding{Code: code, Severity: sev, Message: msg})
	}

	switch {
	case wireBytes >= gmailClipBytes:
		add("gmail_clipping", SeverityError, fmt.Sprintf(
			"This message is %d KB. Gmail clips anything over %d KB, hiding the end of the email and the unsubscribe footer behind a link, and opens stop being recorded for readers who do not expand it.",
			wireBytes/1024, gmailClipBytes/1024))
	case wireBytes >= gmailClipBytes*9/10:
		add("gmail_clipping_near", SeverityWarning, fmt.Sprintf(
			"This message is %d KB, close to the %d KB where Gmail starts clipping. Trimming unused CSS and repeated markup buys the most room.",
			wireBytes/1024, gmailClipBytes/1024))
	}

	doc, err := html.Parse(strings.NewReader(bodyHTML))
	if err != nil {
		return out
	}

	seen := map[atom.Atom]bool{}
	var (
		imagesWithoutAlt int
		externalSheet    bool
		webFont          bool
		hasStyle         bool
		mediaRules       int
		mediaImportant   int
		unsupported      = map[string]bool{}
	)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, bad := strippedElements[n.DataAtom]; bad {
				seen[n.DataAtom] = true
			}
			switch n.DataAtom {
			case atom.Img:
				if strings.TrimSpace(attrOf(n, "alt")) == "" && !isTrackingPixel(n) {
					imagesWithoutAlt++
				}
			case atom.Link:
				if strings.Contains(strings.ToLower(attrOf(n, "rel")), "stylesheet") {
					externalSheet = true
				}
			case atom.Style:
				// Only a sheet this send will actually inline; promising it
				// for one that ships as written would be a lie in the editor.
				hasStyle = hasStyle || sheetEligible(n)
				for _, item := range parseStylesheet(textOf(n)) {
					if item.verbatim == "" {
						noteUnsupported(unsupported, item.decls)
						continue
					}
					lower := strings.ToLower(item.verbatim)
					if strings.HasPrefix(lower, "@font-face") {
						webFont = true
					}
					if strings.HasPrefix(lower, "@media") {
						mediaRules++
						if strings.Contains(lower, "!important") {
							mediaImportant++
						}
					}
				}
			}
			noteUnsupported(unsupported, parseDeclarations(attrOf(n, "style")))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	stripped := make([]string, 0, len(seen))
	for a := range seen {
		stripped = append(stripped, a.String())
	}
	sort.Strings(stripped)
	for _, name := range stripped {
		add(name+"_stripped", SeverityError, strippedElements[atom.Lookup([]byte(name))])
	}
	if externalSheet {
		add("external_stylesheet", SeverityError,
			"A <link rel=\"stylesheet\"> is never fetched by a mail client. Move those rules into a <style> block or onto the elements themselves.")
	}
	if len(unsupported) > 0 {
		props := make([]string, 0, len(unsupported))
		for p := range unsupported {
			props = append(props, p)
		}
		sort.Strings(props)
		add("outlook_unsupported_css", SeverityWarning, fmt.Sprintf(
			"Outlook on Windows renders with Word, which ignores %s. Nest tables and use cell padding for layout that has to hold there.",
			strings.Join(props, ", ")))
	}
	if hasStyle {
		add("stylesheet_inlined", SeverityInfo,
			"The rules in this <style> block are copied onto the elements they match when the email sends, so they survive Outlook.com, Yahoo and Gmail's mobile apps. Media queries and :hover stay in the block.")
	}
	if mediaRules > 0 && mediaImportant == 0 {
		add("media_query_needs_important", SeverityWarning,
			"Media queries stay in the <style> block, but the rules they override have become inline styles, which win. Mark the declarations inside a media query !important for it to take effect.")
	}
	if webFont {
		add("web_font", SeverityInfo,
			"@font-face loads in Apple Mail and some Outlook builds only. Gmail and Outlook on Windows fall back, so put a real font stack after it.")
	}
	if imagesWithoutAlt > 0 {
		add("image_missing_alt", SeverityWarning, fmt.Sprintf(
			"%d image(s) have no alt text. Outlook and Gmail block remote images by default, so alt text is the whole message for a first-time reader.",
			imagesWithoutAlt))
	}
	return out
}

// noteUnsupported collects the declarations Outlook's Word engine drops.
// Properties are matched by name, not by substring: "background-position" is
// not "position", and reporting it would be a false alarm on every template
// that positions a background image.
func noteUnsupported(into map[string]bool, decls []cssDecl) {
	for _, d := range decls {
		if wordUnsupportedProps[d.prop] {
			into[d.prop] = true
		}
		if d.prop == "display" {
			if v := strings.ToLower(d.value); strings.Contains(v, "flex") || strings.Contains(v, "grid") {
				into["display: "+v] = true
			}
		}
	}
}

// isTrackingPixel reports whether an image is a 1x1 beacon, which has nothing
// to say to a screen reader and should not be nagged about.
func isTrackingPixel(n *html.Node) bool {
	w := strings.TrimSpace(attrOf(n, "width"))
	h := strings.TrimSpace(attrOf(n, "height"))
	return (w == "1" && h == "1") || strings.Contains(strings.ToLower(attrOf(n, "style")), "display:none")
}
