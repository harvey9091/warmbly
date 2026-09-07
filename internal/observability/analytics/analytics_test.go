package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// captured is one request the stub PostHog received.
type captured struct {
	APIKey     string         `json:"api_key"`
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Properties map[string]any `json:"properties"`
}

// stub stands in for PostHog's capture API and hands back what it was sent.
func stub(t *testing.T) (*httptest.Server, chan captured) {
	t.Helper()
	got := make(chan captured, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != capturePath {
			t.Errorf("posted to %s, want %s", r.URL.Path, capturePath)
		}
		var c captured
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			t.Errorf("decode body: %v", err)
		}
		got <- c
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func waitFor(t *testing.T, got chan captured) captured {
	t.Helper()
	select {
	case c := <-got:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("no event reached the capture host")
		return captured{}
	}
}

// The contract with PostHog's ingestion: the cookieless sentinel as the
// distinct id, the mode flag, and the three hash inputs. Get any of these
// wrong and the event lands as a separate visitor, or not at all.
func TestCaptureSendsTheCookielessContract(t *testing.T) {
	srv, got := stub(t)
	c := New("phc_test", srv.URL)
	if c == nil {
		t.Fatal("a configured key should produce a client")
	}

	c.Capture("signup_completed", Request{
		IP:        "203.0.113.9",
		UserAgent: "Mozilla/5.0",
		Host:      "app.warmbly.com",
	}, map[string]any{"utm_source": "newsletter"})

	ev := waitFor(t, got)
	if ev.APIKey != "phc_test" {
		t.Errorf("api_key = %q", ev.APIKey)
	}
	if ev.Event != "signup_completed" {
		t.Errorf("event = %q", ev.Event)
	}
	if ev.DistinctID != cookielessDistinctID {
		t.Errorf("distinct_id = %q, want the cookieless sentinel %q", ev.DistinctID, cookielessDistinctID)
	}
	for key, want := range map[string]any{
		"$cookieless_mode": true,
		"$ip":              "203.0.113.9",
		"$raw_user_agent":  "Mozilla/5.0",
		"$host":            "app.warmbly.com",
		"utm_source":       "newsletter",
	} {
		if ev.Properties[key] != want {
			t.Errorf("properties[%q] = %v, want %v", key, ev.Properties[key], want)
		}
	}
}

// Nothing in an event may name a person: that is the whole basis for having no
// consent banner. This guards the property set against a future caller quietly
// adding an identifier.
func TestCaptureCarriesNoIdentifiers(t *testing.T) {
	srv, got := stub(t)
	c := New("phc_test", srv.URL)
	c.Capture("trial_started", Request{IP: "203.0.113.9", Host: "app.warmbly.com"}, nil)

	ev := waitFor(t, got)
	for _, forbidden := range []string{"user_id", "organization_id", "org_id", "email", "distinct_id", "$user_id"} {
		if _, ok := ev.Properties[forbidden]; ok {
			t.Errorf("event carries %q, which would defeat cookieless mode", forbidden)
		}
	}
}

// No key is the self-host default, and it has to mean no client and no request
// rather than a client that quietly points at PostHog Cloud.
func TestNewWithoutAKeyIsOffAndSafeToCall(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if c := New(key, ""); c != nil {
			t.Fatalf("New(%q) returned a client; analytics must be off without a key", key)
		}
	}
	// A nil client is the "never wired" shape and must not panic.
	var c *Client
	c.Capture("signup_completed", Request{}, nil)
}

// An unset host must resolve to EU cloud, and a trailing slash must not produce
// a double slash in the capture path.
func TestHostDefaultingAndTrimming(t *testing.T) {
	if got := New("k", "").host; got != DefaultHost {
		t.Errorf("default host = %q, want %q", got, DefaultHost)
	}
	if got := New("k", "https://ph.example.com/").host; got != "https://ph.example.com" {
		t.Errorf("trailing slash not trimmed: %q", got)
	}
}
