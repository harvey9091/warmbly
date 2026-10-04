package formserver

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// redirectCacheTTL bounds how long a redirect change takes to show; misses are cached the same.
const (
	redirectCacheTTL = time.Minute
	redirectCacheMax = 4096
)

type redirectEntry struct {
	target  string
	expires time.Time
}

// redirects answers "is this host a sending domain's verified root redirect",
// so a form is never served under a domain that only redirects.
type redirects struct {
	client *backendClient
	mu     sync.Mutex
	cache  map[string]redirectEntry
}

func newRedirects(c *backendClient) *redirects {
	return &redirects{client: c, cache: map[string]redirectEntry{}}
}

func (r *redirects) lookup(ctx context.Context, host string) (string, bool) {
	r.mu.Lock()
	if e, ok := r.cache[host]; ok && time.Now().Before(e.expires) {
		r.mu.Unlock()
		return e.target, e.target != ""
	}
	r.mu.Unlock()

	target := ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.client.base+"/api/v1/internal/domain-redirects/"+url.PathEscape(host), nil)
	if err != nil {
		return "", false
	}
	resp, err := r.client.do(req)
	if err != nil {
		// Unknown is not a redirect; the form is served as it would have been.
		return "", false
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			TargetURL string `json:"target_url"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&body); err == nil && safeTarget(body.TargetURL) {
			target = body.TargetURL
		}
	case http.StatusNotFound:
	default:
		return "", false
	}

	r.mu.Lock()
	if len(r.cache) >= redirectCacheMax {
		r.cache = map[string]redirectEntry{}
	}
	r.cache[host] = redirectEntry{target: target, expires: time.Now().Add(redirectCacheTTL)}
	r.mu.Unlock()
	return target, target != ""
}

// safeTarget accepts only an absolute http(s) address.
func safeTarget(t string) bool {
	u, err := url.Parse(t)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(t, "\r\n ")
}

// requestHost is the Host header without port or trailing dot, lower-cased.
func requestHost(raw string) string {
	h := strings.ToLower(strings.TrimSpace(raw))
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.TrimSuffix(h, ".")
}

// domainRedirect sends any request on a verified redirect domain to its target,
// before a form page or submission route can answer under that domain.
func (s *Server) domainRedirect(c *gin.Context) {
	host := requestHost(c.Request.Host)
	if host == "" || !strings.Contains(host, ".") || c.Request.URL.Path == "/health" {
		c.Next()
		return
	}
	if target, ok := s.redirects.lookup(c.Request.Context(), host); ok {
		c.Header("Cache-Control", "public, max-age=300")
		c.Redirect(http.StatusFound, target)
		c.Abort()
		return
	}
	c.Next()
}
