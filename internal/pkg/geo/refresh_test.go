package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureLeavesAFreshDatabaseAloneWithoutAskingTheServer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Write(testDatabase(t))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "GeoLite2-City.mmdb")
	if err := os.WriteFile(path, testDatabase(t), 0o644); err != nil {
		t.Fatal(err)
	}

	if fetched, err := Ensure(context.Background(), path, srv.URL); err != nil || fetched {
		t.Fatalf("Ensure = (%v, %v), want (false, nil)", fetched, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("server was asked %d times about a database written seconds ago", hits.Load())
	}
}

func TestEnsureRevalidatesAStaleDatabaseAndKeepsItOnA304(t *testing.T) {
	var sawConditional atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") != "" {
			sawConditional.Store(true)
			// MaxMind does not charge a 304 against the daily allowance, which
			// is the whole reason staleness checking is affordable.
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write(testDatabase(t))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "GeoLite2-City.mmdb")
	body := testDatabase(t)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	fetched, err := Ensure(context.Background(), path, srv.URL)
	if err != nil || fetched {
		t.Fatalf("Ensure = (%v, %v), want (false, nil)", fetched, err)
	}
	if !sawConditional.Load() {
		t.Fatal("a stale database was re-downloaded instead of revalidated")
	}
	// The file survives, and is restamped so the next boot does not ask again.
	got, err := os.ReadFile(path)
	if err != nil || len(got) != len(body) {
		t.Fatalf("the existing database did not survive the 304: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Fatal("a confirmed-current database was not restamped, so every boot will ask again")
	}
}

func TestEnsureRetriesAThrottledMirrorAndHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// One 429 with guidance, then the database. Before the retry existed
		// this single blip cost the container its geo data permanently.
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write(testDatabase(t))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "GeoLite2-City.mmdb")
	start := time.Now()
	fetched, err := Ensure(context.Background(), path, srv.URL)
	if err != nil || !fetched {
		t.Fatalf("Ensure = (%v, %v), want (true, nil)", fetched, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("server called %d times, want 2", calls.Load())
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("waited %v, ignoring the server's Retry-After of 1s", elapsed)
	}
}

func TestEnsureDoesNotSpendTheAllowanceOnARefusalThatWillRepeat(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		// A bad licence key answers this way every time; asking again only
		// burns more of a 30-a-day budget.
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := Ensure(context.Background(), filepath.Join(t.TempDir(), "db.mmdb"), srv.URL); err == nil {
		t.Fatal("a 401 was treated as success")
	}
	if calls.Load() != 1 {
		t.Fatalf("server called %d times for an unretryable refusal, want 1", calls.Load())
	}
}

func TestAClientAnswersBeforeItsDatabaseArrivesAndAfterItLands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GeoLite2-City.mmdb")
	if err := os.WriteFile(path, testDatabase(t), 0o644); err != nil {
		t.Fatal(err)
	}

	// This is the boot shape: a client with no database at all.
	c, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Lookup(netip.MustParseAddr("8.8.8.8"))
	if err != nil {
		t.Fatalf("a client with no database refused a lookup: %v", err)
	}
	if info.Country != "Unknown" {
		t.Fatalf("country = %q before the database landed, want Unknown", info.Country)
	}

	if err := c.Use(path); err != nil {
		t.Fatalf("Use: %v", err)
	}
	if _, err := c.Lookup(netip.MustParseAddr("8.8.8.8")); err != nil {
		t.Fatalf("lookup after the database landed: %v", err)
	}
	// Swapping again must not leave the previous reader open or panic a
	// concurrent caller.
	if err := c.Use(path); err != nil {
		t.Fatalf("second Use: %v", err)
	}
	c.Close()
}
