package tasks

import (
	"html"
	"strings"

	"github.com/warmbly/warmbly/internal/models"
)

// UnsubscribeLinkVar is the template variable a step can place by hand
// ({{.UnsubscribeLink}}); it resolves to the recipient's own signed link.
const UnsubscribeLinkVar = "UnsubscribeLink"

// unsubscribePathMarker is the path segment every minted link carries. Click
// tracking leaves such links alone so an opt-out is never counted as a click
// or bounced through a redirect.
const unsubscribePathMarker = "/unsubscribe/"

// optOutFooter renders the in-body opt-out for one recipient, as HTML and as
// plain text, or empty strings when the effective mode is off. Link mode with
// no link available (the instance has no public API URL) falls back to the
// text line so a recipient is never left without a way out.
func optOutFooter(settings models.UnsubscribeSettings, linkURL string) (htmlPart, plainPart string) {
	switch settings.Mode {
	case models.UnsubscribeModeOff:
		return "", ""
	case models.UnsubscribeModeLink:
		if linkURL == "" {
			break
		}
		intro := strings.TrimSpace(settings.LinkIntro)
		text := strings.TrimSpace(settings.LinkText)
		htmlPart = `<p style="font-size:12px;color:#64748b;margin-top:16px">` +
			html.EscapeString(intro) + ` <a href="` + html.EscapeString(linkURL) + `" style="color:#64748b">` + html.EscapeString(text) + `</a></p>`
		plainPart = intro + " " + text + ": " + linkURL
		return htmlPart, strings.TrimSpace(plainPart)
	}
	line := strings.TrimSpace(settings.Text)
	if line == "" {
		return "", ""
	}
	return `<p style="font-size:12px;color:#64748b;margin-top:16px">` + html.EscapeString(line) + `</p>`, line
}

// appendOptOut adds the footer after everything else (signature included) so
// it sits where a reader expects an opt-out: last.
func appendOptOut(bodyHTML, bodyPlain string, settings models.UnsubscribeSettings, linkURL string) (string, string) {
	htmlPart, plainPart := optOutFooter(settings, linkURL)
	if htmlPart == "" {
		return bodyHTML, bodyPlain
	}
	if bodyHTML != "" {
		if strings.Contains(bodyHTML, "</body>") {
			bodyHTML = strings.Replace(bodyHTML, "</body>", htmlPart+"</body>", 1)
		} else {
			bodyHTML += htmlPart
		}
	}
	if bodyPlain != "" {
		bodyPlain += "\n\n" + plainPart
	}
	return bodyHTML, bodyPlain
}
