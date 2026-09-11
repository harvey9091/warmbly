package templates

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// ─── Centralized Business Details ────────────────────────────────
// Branding and legal info for every email template, resolved from
// config.Brand() so the footer, the sign-in screen and a public form page all
// say the same thing about who this deployment is.
//
// These are functions, not constants, because a self-hosted install must not
// send mail attributed to Mindroot Ltd with links to someone else's dashboard.
// On a self-host the hosted defaults are not a fallback at all: they are
// another company's registered details and another company's website. Unset
// means unset there, and the footer drops the row rather than filling it with
// ours.

// CompanyName is the product name in subjects and the header. Unlike the legal
// details it is true of a self-host too: it is the software's name.
func CompanyName() string { return config.Brand().Name }

// AppURL is the dashboard base every emailed link is built from.
func AppURL() string { return config.AppBaseURL() }

// footerLink is one entry in the footer's link row. It is a list rather than
// three fixed slots so an install that configured none of them renders no row
// instead of three links to nowhere.
type footerLink struct {
	Label string
	URL   string
}

type baseData struct {
	Subject        string
	Content        template.HTML
	CompanyName    string
	FooterLinks    []footerLink
	LegalLine      string
	RegisteredAddr string
}

// footerLinks is the configured subset of privacy, terms and website.
func footerLinks(b config.BrandConfig) []footerLink {
	var links []footerLink
	for _, l := range []footerLink{
		{Label: "Privacy", URL: b.PrivacyURL},
		{Label: "Terms", URL: b.TermsURL},
		{Label: b.WebsiteLabel(), URL: b.WebsiteURL},
	} {
		if l.URL != "" && l.Label != "" {
			links = append(links, l)
		}
	}
	return links
}

// legalLine is the Companies Act 2006 identification line, which only the
// hosted service is the subject of. A self-host renders no line rather than
// naming a company that has nothing to do with the mail it just sent.
func legalLine(b config.BrandConfig) string {
	parts := []string{}
	for _, p := range []string{b.LegalEntity, b.CompanyNumber, b.PlaceOfReg} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\u00a9 " + strings.Join(parts, " \u00b7 ")
}

var baseTmpl = template.Must(template.New("base").Parse(baseHTML))

func renderEmail(subject, content string) (string, error) {
	// One read of the environment per render, so the header, the links and
	// the legal line can never disagree with each other.
	brand := config.Brand()
	data := baseData{
		Subject:        subject,
		Content:        template.HTML(content),
		CompanyName:    brand.Name,
		FooterLinks:    footerLinks(brand),
		LegalLine:      legalLine(brand),
		RegisteredAddr: brand.Address,
	}
	var buf bytes.Buffer
	if err := baseTmpl.Execute(&buf, data); err != nil {
		errs.CaptureException(err)
		return "", err
	}
	return buf.String(), nil
}

// Dashboard-style transactional email shell.
//
// Replaces the previous radial-blue-gradient marketing-y design with
// the same chrome the user sees in the dashboard:
//   - clean cream background (#f5f6f8),
//   - white card with a hairline #e2e8f0 border,
//   - slate type, no fancy gradients,
//   - slate-900 logo monogram, no decorative haze.
const baseHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1.0"/>
<meta name="color-scheme" content="light"/>
<meta name="supported-color-schemes" content="light"/>
<title>{{.Subject}}</title>
</head>
<body style="margin:0;padding:0;background-color:#f5f6f8;-webkit-text-size-adjust:100%;-ms-text-size-adjust:100%;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#0f172a;">

<table width="100%" cellpadding="0" cellspacing="0" border="0" role="presentation" style="background-color:#f5f6f8;">

<tr>
<td align="center" style="padding:40px 24px 20px;">
<table cellpadding="0" cellspacing="0" border="0" role="presentation">
<tr>
<td valign="middle" style="padding-right:10px;line-height:0;">
<svg width="22" height="22" viewBox="0 0 746 764" fill="none" xmlns="http://www.w3.org/2000/svg" style="display:block;">
<path d="M222.805 644.772L186.274 108.881L704.5 451.158L484.5 451.158L245.5 196.158L444 463.5L222.805 644.772Z" fill="#0f172a"/>
</svg>
</td>
<td valign="middle">
<span style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-weight:700;font-size:15px;color:#0f172a;letter-spacing:-0.01em;">{{.CompanyName}}</span>
</td>
</tr>
</table>
</td>
</tr>

<tr>
<td align="center" style="padding:0 24px;">
<table cellpadding="0" cellspacing="0" border="0" width="520" align="center" role="presentation" style="max-width:520px;width:100%;background-color:#ffffff;border:1px solid #e2e8f0;border-radius:8px;">
<tr>
<td style="padding:32px 36px;">
{{.Content}}
</td>
</tr>
</table>
</td>
</tr>

<tr>
<td align="center" style="padding:32px 24px 48px;">
<table cellpadding="0" cellspacing="0" border="0" role="presentation" style="max-width:520px;width:100%;">
{{if .FooterLinks}}<tr>
<td align="center" style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:11px;line-height:18px;padding-bottom:8px;">
{{range $i, $link := .FooterLinks}}{{if $i}}<span style="color:#cbd5e1;">&nbsp;·&nbsp;</span>
{{end}}<a href="{{$link.URL}}" style="color:#64748b;text-decoration:none;">{{$link.Label}}</a>
{{end}}</td>
</tr>{{end}}
{{if .LegalLine}}<tr>
<td align="center" style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:10px;line-height:16px;color:#94a3b8;">
{{.LegalLine}}
</td>
</tr>{{end}}
{{if .RegisteredAddr}}<tr>
<td align="center" style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:10px;line-height:16px;color:#94a3b8;padding-top:4px;">
{{.RegisteredAddr}}
</td>
</tr>{{end}}
</table>
</td>
</tr>

</table>

</body>
</html>`
