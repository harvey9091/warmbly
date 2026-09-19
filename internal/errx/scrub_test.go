package errx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func renderedBody(t *testing.T, err *Error) (int, response) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Set("request_id", "req-123")

	JSON(c, err)

	var body response
	if uerr := json.Unmarshal(w.Body.Bytes(), &body); uerr != nil {
		t.Fatalf("decode body: %v", uerr)
	}
	return w.Code, body
}

// A 500 must never carry the underlying error text: handlers built those from
// driver output, which names tables and columns.
func TestInternalErrorsAreScrubbed(t *testing.T) {
	status, body := renderedBody(t, New(Internal,
		`ERROR: null value in column "organization_id" of relation "warmup_routing_rules" (SQLSTATE 23502)`))

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if strings.Contains(body.Message, "SQLSTATE") || strings.Contains(body.Message, "organization_id") {
		t.Fatalf("internal detail reached the client: %q", body.Message)
	}
	if body.Message != genericServerMessage {
		t.Fatalf("message = %q, want the generic one", body.Message)
	}
	if body.RequestID != "req-123" {
		t.Fatalf("request id = %q, want it echoed so the detail can be found in the log", body.RequestID)
	}
}

// An Internal-class error explicitly marked public keeps its message: the mail
// transport one is the reason that escape hatch exists.
func TestPublicInternalMessagesSurvive(t *testing.T) {
	_, body := renderedBody(t, ErrMailUndeliverable)
	if body.Message != ErrMailUndeliverable.Message {
		t.Fatalf("message = %q, want the authored one", body.Message)
	}
}

// A message the caller can act on is passed through untouched.
func TestClientErrorsKeepTheirMessage(t *testing.T) {
	for _, err := range []*Error{
		New(BadRequest, "requested value must exceed current effective limit"),
		New(Forbidden, "only the workspace owner can transfer ownership"),
		New(ServiceUnavailable, "No mailbox workers are available right now. Please try again shortly."),
	} {
		_, body := renderedBody(t, err)
		if body.Message != err.Message {
			t.Errorf("message = %q, want %q", body.Message, err.Message)
		}
	}
}
