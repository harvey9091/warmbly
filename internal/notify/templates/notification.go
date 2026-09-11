package templates

import (
	"bytes"
	"html/template"

	"github.com/warmbly/warmbly/internal/observability/errs"
)

// Generic transactional notification used by the in-app notification
// service for any category routed to email (new sign-in, reply alerts,
// and so on). Title/body are plain text, auto-escaped by html/template.
// CTAURL is optional: an empty value drops the button entirely.

const notificationContent = `
<p style="margin:0 0 4px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:11px;color:#94a3b8;letter-spacing:0.14em;text-transform:uppercase;font-weight:500;">
Notification
</p>
<h2 style="margin:0 0 12px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-weight:600;font-size:19px;color:#0f172a;letter-spacing:-0.01em;line-height:1.3;">
{{.Title}}
</h2>
<p style="margin:0 0 20px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;color:#475569;line-height:20px;">
{{.Body}}
</p>
{{if .CTAURL}}
<table cellpadding="0" cellspacing="0" border="0" align="center" role="presentation" style="margin:0 0 24px;">
<tr>
<td align="center" style="border-radius:6px;background:#0f172a;">
<a href="{{.CTAURL}}" target="_blank" style="display:inline-block;padding:10px 22px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;font-weight:600;color:#ffffff;text-decoration:none;letter-spacing:0.01em;">{{.CTALabel}}</a>
</td>
</tr>
</table>
{{end}}
<div style="margin:0 0 16px;height:1px;background:#e2e8f0;"></div>

<p style="margin:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;color:#94a3b8;line-height:18px;">
You're receiving this because email notifications are on for this activity. Manage them in your notification settings.
</p>
`

var notificationTmpl = template.Must(template.New("notification_content").Parse(notificationContent))

// GenerateNotificationHTML renders a generic notification email. ctaURL
// is optional (empty drops the button); ctaLabel defaults to "Open in
// <product name>" when a URL is present but no label is supplied.
func GenerateNotificationHTML(title, body, ctaURL, ctaLabel string) (string, error) {
	brand := CompanyName()
	if ctaURL != "" && ctaLabel == "" {
		ctaLabel = "Open in " + brand
	}
	data := struct {
		Title    string
		Body     string
		CTAURL   string
		CTALabel string
	}{Title: title, Body: body, CTAURL: ctaURL, CTALabel: ctaLabel}
	var buf bytes.Buffer
	if err := notificationTmpl.Execute(&buf, data); err != nil {
		errs.CaptureException(err)
		return "", err
	}
	subject := title
	if subject == "" {
		subject = "Notification from " + brand
	}
	return renderEmail(subject, buf.String())
}
