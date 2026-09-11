package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// proxyTo runs one request through the handler against stand-in upstreams and
// reports which one was dialled and what it saw.
func proxyTo(t *testing.T, method, reqPath string, hdr map[string]string) (hit string, gotPath string, gotHeaders http.Header) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit, gotPath, gotHeaders = "ingest", r.URL.Path, r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ingest.Close()
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit, gotPath, gotHeaders = "assets", r.URL.Path, r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer assets.Close()

	t.Setenv("POSTHOG_PROXY_INGEST", ingest.URL)
	t.Setenv("POSTHOG_PROXY_ASSETS", assets.URL)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/ingest"+reqPath, strings.NewReader(""))
	for k, v := range hdr {
		c.Request.Header.Set(k, v)
	}
	c.Params = gin.Params{{Key: "path", Value: reqPath}}

	h := &Handler{}
	h.PostHogProxy(c)
	return hit, gotPath, gotHeaders
}

// posthog-js fetches its own bundle from the assets host. Sending that to the
// ingestion host returns a 404, which presents as analytics quietly doing
// nothing rather than as an error.
func TestPostHogProxySplitsAssetsFromIngest(t *testing.T) {
	if hit, p, _ := proxyTo(t, http.MethodGet, "/static/array.js", nil); hit != "assets" || p != "/static/array.js" {
		t.Errorf("static went to %q path %q, want assets /static/array.js", hit, p)
	}
	if hit, p, _ := proxyTo(t, http.MethodPost, "/e/", nil); hit != "ingest" || p != "/e/" {
		t.Errorf("capture went to %q path %q, want ingest /e/", hit, p)
	}
}

// The upstream is never taken from the request, and the path is cleaned before
// it is appended, so a crafted path cannot reach anything else.
func TestPostHogProxyRefusesTraversal(t *testing.T) {
	_, p, _ := proxyTo(t, http.MethodGet, "/../../admin/secrets", nil)
	if strings.Contains(p, "..") {
		t.Errorf("traversal survived into the upstream path: %q", p)
	}
	if !strings.HasPrefix(p, "/") {
		t.Errorf("upstream path is not rooted: %q", p)
	}
}

// Nothing that authenticates a caller to THIS instance belongs in a request to
// a third party.
func TestPostHogProxyWithholdsCredentials(t *testing.T) {
	_, _, hdr := proxyTo(t, http.MethodPost, "/e/", map[string]string{
		"Cookie":        "warmbly_session=secret",
		"Authorization": "Bearer internal-token",
		"Content-Type":  "application/json",
	})
	if hdr.Get("Cookie") != "" {
		t.Error("the caller's cookies were forwarded to PostHog")
	}
	if hdr.Get("Authorization") != "" {
		t.Error("the caller's Authorization header was forwarded to PostHog")
	}
	if hdr.Get("Content-Type") != "application/json" {
		t.Error("Content-Type was dropped; PostHog cannot parse the body without it")
	}
	if hdr.Get("X-Forwarded-For") == "" {
		t.Error("no client address forwarded; PostHog would geolocate the server")
	}
}
