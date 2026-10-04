package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/app/contact"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

type recordingContacts struct {
	contact.ContactService
	added []models.AddContact
}

func (r *recordingContacts) Add(_ context.Context, _ string, _ uuid.UUID, in []models.AddContact) ([]models.Contact, *errx.Error) {
	r.added = in
	out := make([]models.Contact, len(in))
	return out, nil
}

func postContacts(t *testing.T, body string) (*httptest.ResponseRecorder, *recordingContacts) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := &recordingContacts{}
	h := &Handler{ContactService: svc}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/contacts", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.OrganizationIDKey, uuid.New())
	h.AddContacts(c)
	return w, svc
}

// The CLIs create one contact by sending a bare object, so it has to be accepted alongside the array (issue #650).
func TestAddContactsAcceptsOneObjectOrAnArray(t *testing.T) {
	for _, body := range []string{
		`{"email":"jane@example.com","first_name":"Jane"}`,
		` [{"email":"jane@example.com","first_name":"Jane"}]`,
	} {
		w, svc := postContacts(t, body)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d, body %s", body, w.Code, w.Body.String())
		}
		if len(svc.added) != 1 || svc.added[0].Email != "jane@example.com" || svc.added[0].FirstName != "Jane" {
			t.Fatalf("%s: service got %+v", body, svc.added)
		}
		var out []json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out) != 1 {
			t.Fatalf("%s: response is not a one-item array: %s", body, w.Body.String())
		}
	}
}

func TestAddContactsNamesWhatIsWrongWithTheBody(t *testing.T) {
	cases := map[string]string{
		``:                    "request body is empty",
		`[]`:                  "no contacts provided",
		`{"email":5}`:         `Field "email" must be a JSON string, not a JSON number`,
		`"jane@example.com"`:  "must be a JSON array, not a JSON string",
		`{"email":"a@b.co",}`: "not valid JSON",
	}
	for body, want := range cases {
		w, svc := postContacts(t, body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%q: status %d, want 400", body, w.Code)
		}
		if svc.added != nil {
			t.Fatalf("%q: reached the service", body)
		}
		var resp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if !strings.Contains(resp.Message, want) {
			t.Errorf("%q: message %q does not say %q", body, resp.Message, want)
		}
	}
}
