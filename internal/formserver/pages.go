package formserver

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// The embed loader ships inside the binary, like the tracking snippet does.
//
//go:embed static/forms-embed.js
var formsEmbedJS []byte

// Terminal pages for a shell request that cannot reach the app. The React
// app renders the same copy for its own in-app failure states.

func formNotFoundPage(c *gin.Context) {
	c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>Form not found</title></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:26rem;margin:5rem auto;padding:0 1rem;color:#0f172a;text-align:center"><h1 style="font-size:1.1rem;margin:0 0 .5rem">This form is no longer available</h1><p style="font-size:.9rem;color:#475569">It may have been unpublished or removed.</p></body></html>`))
}

func formUnavailablePage(c *gin.Context) {
	c.Data(http.StatusServiceUnavailable, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>Form unavailable</title></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:26rem;margin:5rem auto;padding:0 1rem;color:#0f172a;text-align:center"><h1 style="font-size:1.1rem;margin:0 0 .5rem">This form is temporarily unavailable</h1><p style="font-size:.9rem;color:#475569">Please try again in a moment.</p></body></html>`))
}

// rootPage answers the bare host. A form is reached through its own
// unguessable link, so there is nothing here to list; a styled 200 is still
// better than gin's plain-text 404, which reads as a broken deployment to
// anyone who types the domain and to any uptime check pointed at `/`.
//
// No vendor name and no brand: this host is the customer's, and the only
// branded surface is the form page itself.
func rootPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>Forms</title></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:26rem;margin:5rem auto;padding:0 1rem;color:#0f172a;text-align:center"><h1 style="font-size:1.1rem;margin:0 0 .5rem">This address serves hosted forms</h1><p style="font-size:.9rem;color:#475569">Each form has its own link. Open the one you were sent to fill it in.</p></body></html>`))
}

// notFoundPage is the same page shape for any other unknown path, so a typo in
// a form link does not land on a bare framework error.
func notFoundPage(c *gin.Context) {
	c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>Not found</title></head><body style="font-family:system-ui,-apple-system,sans-serif;max-width:26rem;margin:5rem auto;padding:0 1rem;color:#0f172a;text-align:center"><h1 style="font-size:1.1rem;margin:0 0 .5rem">There is nothing at this address</h1><p style="font-size:.9rem;color:#475569">Check the link you were sent.</p></body></html>`))
}

// setFormFrameHeaders enforces the embed allowlist in the browser: with
// domains set, only those sites (and the form's own origin) may frame it.
// This is why the shell must come from this process and not a dumb file
// server: the header is per-form.
func setFormFrameHeaders(c *gin.Context, allowedDomains []string) {
	// An empty allowlist means "any site may embed this", which is the
	// documented contract and what every embed installed without configuring
	// the list depends on. Narrowing it to 'self' would take those forms off
	// their owners' websites with no error anywhere, so the default stays.
	//
	// What changed is that the policy is now stated rather than absent. A
	// missing header and a permissive one behave the same in a browser, but
	// only one of them says which it meant, and a scanner cannot tell an
	// intentional default from an oversight.
	//
	// An owner who wants framing restricted lists their domains, which is the
	// control the product actually offers.
	if len(allowedDomains) == 0 {
		c.Header("Content-Security-Policy", "frame-ancestors *")
		return
	}

	sources := make([]string, 0, len(allowedDomains)*2+1)
	sources = append(sources, "'self'")
	for _, d := range allowedDomains {
		sources = append(sources, d, "*."+d)
	}
	c.Header("Content-Security-Policy", "frame-ancestors "+strings.Join(sources, " "))
}
