package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersAreSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-site",
	}
	for k, v := range want {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("a default Content-Security-Policy should be set")
	}
}

// HSTS over plain HTTP is ignored by browsers and would pin a self-hosted LAN
// instance to a scheme it does not serve, so it is only sent on a TLS request.
func TestHSTSOnlyOverTLS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	plain := httptest.NewRecorder()
	r.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/x", nil))
	if got := plain.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("no HSTS expected over plain HTTP, got %q", got)
	}

	forwarded := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(forwarded, req)
	if got := forwarded.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("HSTS expected when the edge reports https")
	}
}

// A handler that sets its own policy keeps it: the OAuth bouncer pages need
// script-src 'unsafe-inline' for the one script that is the page.
func TestHandlerCSPWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/page", func(c *gin.Context) {
		c.Header("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/page", nil))
	if got := w.Header().Get("Content-Security-Policy"); got != "default-src 'none'; script-src 'unsafe-inline'" {
		t.Errorf("handler policy should survive, got %q", got)
	}
}
