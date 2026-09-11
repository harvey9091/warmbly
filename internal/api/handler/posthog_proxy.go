package handler

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PostHog reverse proxy. Analytics and error reporting are blocked by most
// content blockers when the browser talks to posthog.com directly, so the
// client is pointed at this instance instead and the request is forwarded from
// here. Nothing about the payload changes; only who the browser dials.
//
// It lives on the backend rather than in each frontend because there are three
// of them (dashboard, admin, marketing) and a proxy copied three times drifts
// three ways. A self-host also always has a backend, and may well have no CDN
// to put a function on.
//
// Point the frontends at it with POSTHOG_HOST (dashboard and admin) or
// PUBLIC_POSTHOG_HOST (marketing) set to https://<api host>/ingest.

const (
	// Ingestion and asset traffic are different hosts upstream: posthog-js
	// fetches its own bundle from the assets host, and sending that to the
	// ingestion host returns a 404 that presents as "analytics silently does
	// nothing".
	posthogIngestDefault = "https://us.i.posthog.com"
	posthogAssetsDefault = "https://us-assets.i.posthog.com"

	posthogProxyTimeout = 30 * time.Second
)

// posthogUpstream picks the host for one request path. Only these two are ever
// dialled: the upstream is never taken from the request, so this cannot be
// turned into an open relay.
func (h *Handler) posthogUpstream(reqPath string) string {
	ingest, assets := posthogIngestDefault, posthogAssetsDefault
	if v := strings.TrimRight(os.Getenv("POSTHOG_PROXY_INGEST"), "/"); v != "" {
		ingest = v
	}
	if v := strings.TrimRight(os.Getenv("POSTHOG_PROXY_ASSETS"), "/"); v != "" {
		assets = v
	}
	if strings.HasPrefix(reqPath, "/static/") {
		return assets
	}
	return ingest
}

// PostHogProxy forwards /ingest/* to PostHog.
func (h *Handler) PostHogProxy(c *gin.Context) {
	// path.Clean resolves any ".." before it can be appended to the upstream,
	// so a crafted path cannot walk out of the prefix it is allowed to reach.
	raw := "/" + strings.TrimPrefix(c.Param("path"), "/")
	suffix := path.Clean(raw)
	// Clean also drops a trailing slash, and PostHog's capture endpoint is
	// "/e/". Losing it turns every captured event into a 404 that looks like
	// analytics simply not working.
	if strings.HasSuffix(raw, "/") && !strings.HasSuffix(suffix, "/") {
		suffix += "/"
	}

	target, err := url.Parse(h.posthogUpstream(suffix) + suffix)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	target.RawQuery = c.Request.URL.RawQuery

	body := io.LimitReader(c.Request.Body, 10<<20)
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target.String(), body)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}

	// Only what PostHog needs to interpret the request. The inbound Host,
	// cookies and Authorization stay here: none belongs to posthog.com, and
	// forwarding them would hand a third party this instance's session.
	for _, k := range []string{"Content-Type", "Content-Encoding", "Accept", "Accept-Encoding", "User-Agent", "X-Requested-With"} {
		if v := c.GetHeader(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	// The caller's address, so PostHog geolocates the visitor rather than this
	// server. Set rather than appended: an inbound value is attacker-controlled.
	req.Header.Set("X-Forwarded-For", c.ClientIP())

	client := &http.Client{Timeout: posthogProxyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for _, k := range []string{"Content-Type", "Content-Encoding", "Cache-Control", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
