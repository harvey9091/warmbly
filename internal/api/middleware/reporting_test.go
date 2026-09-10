package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// A panic has to answer the request the same way gin's own recovery did, or
// swapping the two turns a 500 into a hung connection.
func TestRecoveryStillReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery())
	r.GET("/boom", func(*gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// The tags are the whole point of reporting a panic rather than logging it: an
// issue nobody can tie to a route, a request id or a workspace is an issue
// nobody can act on.
func TestRequestTagsCarryTheIds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orgID := uuid.New()

	var tags map[string]string
	r := gin.New()
	r.GET("/v1/campaigns/:id", func(c *gin.Context) {
		c.Set(RequestIDContextKey, "req-123")
		c.Set(OrganizationIDKey, orgID)
		c.Set(UserIDKey, "user-abc")
		tags = requestTags(c)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/campaigns/"+uuid.NewString()+"?token=secret", nil))

	want := map[string]string{
		"http_method": http.MethodGet,
		// The pattern, not the path: one issue per route, and the id in the
		// URL never becomes part of the issue.
		"http_route":      "/v1/campaigns/:id",
		"request_id":      "req-123",
		"organization_id": orgID.String(),
		"user_id":         "user-abc",
	}
	if len(tags) != len(want) {
		t.Fatalf("tags = %v, want exactly %v", tags, want)
	}
	for key, value := range want {
		if tags[key] != value {
			t.Errorf("%s = %q, want %q", key, tags[key], value)
		}
	}
}

// An unmatched path is attacker-supplied, so it must not reach the report and
// mint one issue per probe. An unauthenticated request carries no ids either.
func TestRequestTagsOnAnUnmatchedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/wp-admin/setup-config.php", nil)

	tags := requestTags(c)
	if tags["http_route"] != "unmatched" {
		t.Errorf("http_route = %q, want unmatched", tags["http_route"])
	}
	for _, key := range []string{"request_id", "organization_id", "user_id"} {
		if _, ok := tags[key]; ok {
			t.Errorf("%s was tagged on an unauthenticated request", key)
		}
	}
}
