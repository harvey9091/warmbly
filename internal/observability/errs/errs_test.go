package errs

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// A deployment that configured no backend is the self-host default, so that
// path has to keep working on its own: the event reaches the process log and
// nothing leaves the machine.
func TestNoBackendReportsToTheLog(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := Init(Config{Service: "backend", Environment: "dev"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	CaptureException(errors.New("boom"))
	CaptureMessage("starting")
	Recover("panicked")

	got := buf.String()
	for _, want := range []string{"[issue-local][backend][error] boom", "[issue-local][backend][info] starting", "[issue-local][backend][panic] panicked"} {
		if !strings.Contains(got, want) {
			t.Errorf("log is missing %q, got %s", want, got)
		}
	}
}

// The one thing worth asserting about the PostHog backend is the shape of what
// it posts: PostHog reads an exception out of $exception_list and nothing else,
// so an event that carries the error anywhere else is an event that never
// becomes an issue.
func TestPostHogPostsAnException(t *testing.T) {
	bodies := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	t.Cleanup(srv.Close)

	if err := Init(Config{
		PostHogKey:  "phc_test",
		PostHogHost: srv.URL,
		Service:     "worker",
		Environment: "prod",
		Release:     "v1.2.3",
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	CaptureException(errors.New("boom"), Tag("mailbox_id", "m-1"))
	// Flushing closes the client, which is why this is the last thing the test
	// asks of it.
	Flush(5 * time.Second)

	var body string
	select {
	case body = <-bodies:
	case <-time.After(5 * time.Second):
		t.Fatal("nothing reached the capture host")
	}

	for _, want := range []string{
		`"$exception"`,
		`"$exception_list"`,
		`"boom"`,
		// The tag the caller attached, flattened into the event properties.
		`"mailbox_id"`,
		// The process, not a person, and no profile built for it.
		`"warmbly-worker"`,
		`"$process_person_profile":false`,
		// The frame the capture came from, so the issue groups by call site.
		`errs.TestPostHogPostsAnException`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("captured body is missing %s: %s", want, body)
		}
	}
}
