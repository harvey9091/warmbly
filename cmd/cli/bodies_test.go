package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/app/emailsend"
	"github.com/warmbly/warmbly/internal/cli/iostreams"
	"github.com/warmbly/warmbly/internal/models"
)

type capturedRequest struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

// runAgainst runs one CLI invocation against a recording server and returns the request it made.
func runAgainst(t *testing.T, args ...string) capturedRequest {
	t.Helper()
	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query = r.Method, r.URL.Path, r.URL.Query()
		got.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	t.Setenv("WARMBLY_CONFIG_DIR", t.TempDir())
	t.Setenv("WARMBLY_TOKEN", "wmbly_test")
	t.Setenv("WARMBLY_API_URL", srv.URL)
	var out bytes.Buffer
	f := &Factory{IO: &iostreams.IOStreams{In: strings.NewReader(""), Out: &out, ErrOut: &out}}
	root := newRootCmd(f)
	root.SetArgs(append(args, "--yes", "--json"))
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("warmbly %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	if got.method == "" {
		t.Fatalf("warmbly %s made no request", strings.Join(args, " "))
	}
	return got
}

// strictDecode fails when the body carries a key the server's struct does not read.
func strictDecode(t *testing.T, body []byte, dst any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("body %s does not decode into %T: %v", body, dst, err)
	}
}

func bodyKeys(t *testing.T, body []byte) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body %s is not an object: %v", body, err)
	}
	return m
}

// Each command's body has to be one the endpoint binds (issue #650).
func TestCommandBodiesMatchTheAPI(t *testing.T) {
	t.Run("contact create", func(t *testing.T) {
		r := runAgainst(t, "contact", "create", "--email", "jane@example.com", "--first-name", "Jane")
		var c models.AddContact
		strictDecode(t, r.body, &c)
		if c.Email != "jane@example.com" || c.FirstName != "Jane" {
			t.Fatalf("decoded %+v", c)
		}
	})

	t.Run("mailbox send", func(t *testing.T) {
		r := runAgainst(t, "mailbox", "send", "MB", "--to", "a@example.com,b@example.com", "--subject", "Hi", "--body", "<p>Hi</p>")
		var req emailsend.SendEmailRequest
		strictDecode(t, r.body, &req)
		if len(req.To) != 2 || req.Subject != "Hi" || req.BodyHTML != "<p>Hi</p>" {
			t.Fatalf("decoded %+v", req)
		}
	})

	t.Run("mailbox set-tracking", func(t *testing.T) {
		r := runAgainst(t, "mailbox", "set-tracking", "MB", "--domain", "t.example.com")
		if r.method != http.MethodPatch || r.query.Get("domain") != "t.example.com" || len(r.body) != 0 {
			t.Fatalf("sent %s ?%s body %q", r.method, r.query.Encode(), r.body)
		}
	})

	t.Run("form set-domain", func(t *testing.T) {
		m := bodyKeys(t, runAgainst(t, "form", "set-domain", "--domain", "f.example.com").body)
		if m["forms_domain"] != "f.example.com" || len(m) != 1 {
			t.Fatalf("sent %v", m)
		}
	})

	t.Run("campaign test", func(t *testing.T) {
		m := bodyKeys(t, runAgainst(t, "campaign", "test", "C", "--mailbox", "MB", "--to", "you@example.com").body)
		if m["recipient"] != "you@example.com" || m["account_id"] != "MB" || len(m) != 2 {
			t.Fatalf("sent %v", m)
		}
	})

	t.Run("task create", func(t *testing.T) {
		r := runAgainst(t, "task", "create", "--title", "Call", "--due", "2026-10-01T09:00:00Z")
		var task models.CreateCRMTask
		strictDecode(t, r.body, &task)
		if task.DueDate == nil || task.Title != "Call" {
			t.Fatalf("decoded %+v", task)
		}
	})

	t.Run("advisor snooze", func(t *testing.T) {
		var req models.AdvisorSnoozeRequest
		strictDecode(t, runAgainst(t, "advisor", "snooze", "R", "--days", "7").body, &req)
		if req.Days != 7 {
			t.Fatalf("decoded %+v", req)
		}
	})

	t.Run("mailbox warmup-appeal", func(t *testing.T) {
		m := bodyKeys(t, runAgainst(t, "mailbox", "warmup-appeal", "MB", "--reason", "fixed DNS").body)
		if m["reason"] != "fixed DNS" || len(m) != 1 {
			t.Fatalf("sent %v", m)
		}
	})

	t.Run("integration push", func(t *testing.T) {
		var sel models.ContactSelection
		strictDecode(t, runAgainst(t, "integration", "push", "C", "--contacts", "a,b").body, &sel)
		if len(sel.Contacts) != 2 {
			t.Fatalf("decoded %+v", sel)
		}
	})

	t.Run("oauth-app create", func(t *testing.T) {
		var w models.OAuthApplicationWrite
		strictDecode(t, runAgainst(t, "oauth-app", "create", "--name", "App", "--scopes", "3").body, &w)
		if w.Scopes != 3 || w.Name != "App" {
			t.Fatalf("decoded %+v", w)
		}
	})
}
