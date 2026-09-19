package errx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJSONIncludesStableCodeAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("request_id", "req_test_123")

	JSON(c, New(BadRequest, "invalid cursor"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "bad_request" {
		t.Fatalf("code = %q", body.Code)
	}
	if body.RequestID != "req_test_123" {
		t.Fatalf("request_id = %q", body.RequestID)
	}
	if body.Error != "Bad Request" || body.Message != "invalid cursor" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

// A Code outside the table used to map to status 0, which gin leaves at 200.
// Every error carrying an upstream status (the Warmbly Cloud client is the one
// that does) would then be reported to the caller as a success.
func TestJSONAnswersAnUnknownCodeAsAnError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	JSON(c, New(Code(599), "upstream fell over"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "internal_error" {
		t.Fatalf("code = %q", body.Code)
	}
}

// A 5xx written for a stack trace is not what the caller is told. The call
// site's own words used to be the explanation under a failed action in the
// dashboard: "failed to get organization count" is not something anybody can
// act on, and it names internals a caller should not have to know.
func TestJSONKeepsAnInternalMessageOffTheWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("request_id", "req_test_500")

	JSON(c, New(Internal, "failed to get organization count"))

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if strings.Contains(body.Message, "organization count") {
		t.Fatalf("internal message reached the caller: %q", body.Message)
	}
	if body.Message != answer5xx(Internal) {
		t.Fatalf("message = %q", body.Message)
	}
	// The id is how the sentence above turns into something an operator can
	// look up, so it has to survive the substitution.
	if body.RequestID != "req_test_500" {
		t.Fatalf("request_id = %q", body.RequestID)
	}
}

// A 5xx the reader can act on keeps its own words. A self-hoster with no mail
// transport configured learns nothing from a generic fault.
func TestJSONKeepsAPublicInternalMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	JSON(c, ErrMailUndeliverable)

	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Message != ErrMailUndeliverable.Message {
		t.Fatalf("message = %q", body.Message)
	}
}

// A 4xx is written for the person reading it at every call site, so nothing
// replaces it.
func TestUserMessagePassesClientErrorsThrough(t *testing.T) {
	if got := New(BadRequest, "Limit must be between 10 and 200.").UserMessage(); got != "Limit must be between 10 and 200." {
		t.Fatalf("UserMessage() = %q", got)
	}
	if got := NewWithIdentifier(ServiceUnavailable, "mailbox_worker_unreachable", "Try again in a moment.").UserMessage(); got != "Try again in a moment." {
		t.Fatalf("identified 5xx was replaced: %q", got)
	}
}
