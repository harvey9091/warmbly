package typesafe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// `detail` arrives in three different shapes. Each of these bodies was copied
// from a real response, not invented: a parser written for any one of them
// turns the other two into an empty error message at exactly the moment
// somebody is trying to find out what went wrong.
func TestParseDetailAllThreeShapes(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		contains string
	}{
		{
			// 400 unknown model, and 401 bad key, use this shape.
			name:     "object",
			body:     `{"detail":{"error_type":"api_usage_error","message":"Unknown model: jev-9.9.9"}}`,
			contains: "Unknown model",
		},
		{
			// 400 schema violation: an eleven-level score question.
			name:     "string",
			body:     `{"detail":"Too many score levels. Must have at most 10 levels."}`,
			contains: "at most 10 levels",
		},
		{
			// 422 request validation.
			name:     "array",
			body:     `{"detail":[{"type":"union_tag_invalid","loc":["body","questions","a"],"msg":"Input tag 'bogus' found using 'type' does not match any of the expected tags","input":{"type":"bogus"}}]}`,
			contains: "does not match any of the expected tags",
		},
		{
			name:     "not json at all",
			body:     `<html>502 Bad Gateway</html>`,
			contains: "502",
		},
		{
			name:     "empty",
			body:     ``,
			contains: "no detail",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDetail([]byte(tc.body))
			if !strings.Contains(got, tc.contains) {
				t.Fatalf("parseDetail(%s) = %q, want it to contain %q", tc.name, got, tc.contains)
			}
		})
	}
}

func testQuestions() map[string]Question {
	return map[string]Question{
		"kind": Choice("What is this message?", map[string]string{"human_reply": "A person replying"}),
	}
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient("test-key")
	c.endpoint = srv.URL
	// Do not spend real seconds proving a backoff happened.
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

// 429 and 529 are the two statuses that mean "try again". Everything else is a
// mistake in the request and retrying it just makes the same mistake.
func TestRetriesOn429ThenSucceeds(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"kind":{"type":"choice","choice":"human_reply","confidence":0.9,"probabilities":{"human_reply":0.9}}},"usage":{"input_tokens":10,"output_tokens":2}}`))
	}))
	defer srv.Close()

	resp, err := testClient(t, srv).Ask(context.Background(), "x", testQuestions())
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("made %d attempts, want 3", attempts)
	}
	if resp.Answers["kind"].Choice != "human_reply" {
		t.Fatalf("answer not decoded: %+v", resp.Answers)
	}
}

func TestRetriesOn529(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"detail":{"error_type":"overloaded","message":"TypeSafe is temporarily overloaded"}}`))
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Ask(context.Background(), "x", testQuestions())
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if attempts != 4 {
		t.Fatalf("made %d attempts, want the full 4", attempts)
	}
	if !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("error lost the server's message: %v", err)
	}
}

// A 400 is a bug in the request. Retrying it four times turns one wrong call
// into four wrong calls and delays the error by the whole backoff.
func TestDoesNotRetryOn400(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"Too many score levels. Must have at most 10 levels."}`))
	}))
	defer srv.Close()

	_, err := testClient(t, srv).Ask(context.Background(), "x", testQuestions())
	if err == nil {
		t.Fatal("expected an error")
	}
	if attempts != 1 {
		t.Fatalf("made %d attempts on a 400, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "at most 10 levels") {
		t.Errorf("error lost the detail: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("7"); got != 7*time.Second {
		t.Errorf("delta-seconds: got %v, want 7s", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty: got %v, want 0", got)
	}
	if got := parseRetryAfter("not a number"); got != 0 {
		t.Errorf("garbage: got %v, want 0", got)
	}
	// An HTTP date in the past must not produce a negative wait.
	if got := parseRetryAfter("Mon, 02 Jan 2006 15:04:05 GMT"); got != 0 {
		t.Errorf("past date: got %v, want 0", got)
	}
}

// Every score question must be inside the API's ten-level ceiling. Eleven is a
// 400, and it would only be discovered in production on whichever message first
// reached that question.
func TestScoreQuestionsWithinAPILimit(t *testing.T) {
	for id, q := range testQuestions() {
		if q.Type != QuestionScore {
			continue
		}
		levels, ok := q.Criteria.([]string)
		if !ok {
			t.Fatalf("%s: score criteria is not an ordered list", id)
		}
		if len(levels) < 2 {
			t.Errorf("%s: %d levels, the API requires at least 2", id, len(levels))
		}
		if len(levels) > 10 {
			t.Errorf("%s: %d levels, the API rejects more than 10", id, len(levels))
		}
	}
}

// The model is pinned. An alias moves, and every threshold in policy.go was
// calibrated against this exact version.
func TestModelIsPinned(t *testing.T) {
	if strings.Contains(Model, "latest") {
		t.Fatalf("model %q is an alias; pin an exact version", Model)
	}
	if Model != "jev-1.13.0" {
		t.Fatalf("model changed to %q: re-check every threshold in policy.go before updating this test", Model)
	}
}
