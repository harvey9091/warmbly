package errs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Repeat suppression (repeat.go) is process-wide, so one test reporting the
// same error as another would be silenced by it. Every test below starts from
// an empty table.
func freshReports(t *testing.T) {
	t.Helper()
	repeatMu.Lock()
	repeats = map[string]*repeatState{}
	repeatMu.Unlock()
}

// A deployment that configured no backend is the self-host default, so that
// path has to keep working on its own: the event reaches the process log and
// nothing leaves the machine.
func TestNoBackendReportsToTheLog(t *testing.T) {
	freshReports(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := Init(Config{Service: "backend", Environment: "dev"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	CaptureException(errors.New("boom"))
	CaptureMessage("starting")
	Recover("panicked")

	// A caller that reports unconditionally must not mint an issue with
	// nothing in it.
	CaptureException(nil)
	CaptureMessage("")
	Recover(nil)

	got := buf.String()
	if strings.Count(got, "[issue-local]") != 3 {
		t.Errorf("an empty report was logged: %s", got)
	}
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
	freshReports(t)
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

// A rolling restart cancels every request in flight, and each one unwound
// through a repository that reported the failed query. Nine issues naming
// whichever statements happened to be running is not a deploy anybody needs
// told about, so a cancelled context must not reach a backend at all. A
// deadline this process set still must.
func TestCancelledWorkIsNotReported(t *testing.T) {
	freshReports(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := Init(Config{Service: "backend", Environment: "dev"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	CaptureException(context.Canceled)
	// Wrapped the way a repository reports it, through the query text.
	CaptureException(fmt.Errorf("queryrow failed: %w", context.Canceled))
	CaptureExceptionContext(ctx, fmt.Errorf("get organization: %w", context.Canceled))

	if strings.Contains(buf.String(), "[issue-local]") {
		t.Errorf("a cancelled request was reported: %s", buf.String())
	}

	CaptureException(fmt.Errorf("queryrow failed: %w", context.DeadlineExceeded))
	if !strings.Contains(buf.String(), "[issue-local]") {
		t.Error("a deadline this process set and blew through was dropped as if the caller had gone away")
	}
}

// One fault that recurs is one thing to fix. A schema the registry refused
// filed 19,190 events in three hours and a cache provider over its request
// quota filed 2,815 in five, and in both cases every other issue in the
// project was pushed off the first page while nothing was learned after the
// first copy. So the second sighting is counted rather than sent, and the
// report that ends the silence says how many it stands for.
func TestARecurringFaultReportsOnce(t *testing.T) {
	freshReports(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := Init(Config{Service: "backend", Environment: "dev"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for i := 0; i < 50; i++ {
		CaptureException(fmt.Errorf("register w.%d-value: schema registry refused", i))
	}
	if got := strings.Count(buf.String(), "[issue-local]"); got != 1 {
		t.Fatalf("reported %d times, want 1", got)
	}

	// A different fault is a different thing to fix and is never held back by
	// one that happens to be recurring.
	CaptureException(errors.New("the mail server closed the connection"))
	if got := strings.Count(buf.String(), "[issue-local]"); got != 2 {
		t.Fatalf("a distinct fault was suppressed: %d reports", got)
	}
}

// The window is what makes the suppression temporary rather than permanent: a
// fault still recurring after it reports again, carrying what it stood for.
func TestSuppressionEndsWithTheWindow(t *testing.T) {
	freshReports(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := Init(Config{Service: "backend", Environment: "dev"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := errors.New("redis: max requests limit exceeded")
	CaptureException(err)
	CaptureException(err)
	CaptureException(err)

	// Age the streak past the window rather than waiting it out.
	repeatMu.Lock()
	for _, state := range repeats {
		state.reportedAt = state.reportedAt.Add(-repeatWindow - time.Second)
	}
	repeatMu.Unlock()

	var sc scope
	if !admit(&sc, fingerprintError(err)) {
		t.Fatal("a fault still recurring after the window stayed suppressed")
	}
	if sc.extra["repeat.suppressed"] != 2 {
		t.Fatalf("suppressed count = %v, want 2", sc.extra["repeat.suppressed"])
	}
}

// The last thing a process says before it exits is exempt: a crash loop
// restarting every few seconds would otherwise report its first boot failure
// and go quiet through every one after it.
func TestAFatalIsNeverSuppressed(t *testing.T) {
	freshReports(t)
	var sc scope
	Always()(&sc)
	key := fingerprintError(errors.New("boot failed"))
	if !admit(&sc, key) || !admit(&sc, key) {
		t.Fatal("an always-report event was suppressed")
	}
}

// The key collapses the ids and addresses that differ between sightings of one
// fault, and keeps apart two faults that differ in anything else.
func TestFingerprintGroupsOneFaultAndSeparatesTwo(t *testing.T) {
	a := fingerprintError(fmt.Errorf("mailbox 7c2f1f0e-1111-4a1b-8f01-000000000001 could not be reached"))
	b := fingerprintError(fmt.Errorf("mailbox 9d3a2b1c-2222-4c2d-9e02-000000000002 could not be reached"))
	if a != b {
		t.Fatalf("one fault split by its ids:\n%s\n%s", a, b)
	}
	if a == fingerprintError(fmt.Errorf("mailbox 7c2f1f0e-1111-4a1b-8f01-000000000001 refused the password")) {
		t.Fatal("two faults collapsed into one key")
	}
}
