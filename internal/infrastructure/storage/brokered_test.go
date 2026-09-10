package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// brokerFor stands up a fake control plane and a fake object store, wired the
// way the real pair is: the broker hands back a URL into the store, and the
// node is expected to talk to the store and not to the broker.
func brokerFor(t *testing.T, store http.Handler) (*BrokeredStore, *int) {
	t.Helper()
	objects := httptest.NewServer(store)
	t.Cleanup(objects.Close)

	presignCalls := 0
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req presignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		presignCalls++
		op := PresignOp(req.Op)
		if !op.Valid() {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(presignResponse{
			URL:    objects.URL + "/" + req.Key,
			Method: op.Method(),
		})
	}))
	t.Cleanup(broker.Close)

	s, err := NewBrokered(broker.URL, "tok")
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	return s, &presignCalls
}

func TestBrokeredGet(t *testing.T) {
	s, calls := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("object store saw %s, want GET", r.Method)
		}
		_, _ = io.WriteString(w, "body bytes")
	}))

	rc, err := s.Get(context.Background(), "emails/1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "body bytes" {
		t.Errorf("got %q, want %q", got, "body bytes")
	}
	if *calls != 1 {
		t.Errorf("presign called %d times, want 1", *calls)
	}
}

// A URL signed for one key answers a missing object with 404.
func TestBrokeredGetMissingIsNotFound(t *testing.T) {
	s, _ := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if _, err := s.Get(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// A 403 is an expired signature or a credential that lost the bucket, not a
// missing object. Reporting it as ErrNotFound turns a config error into "the
// body is gone" on every send, and sends the operator looking for the wrong
// thing entirely.
func TestBrokeredRefusedIsNotMissing(t *testing.T) {
	s, _ := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := s.Get(context.Background(), "k")
	if err == nil {
		t.Fatal("a 403 was accepted")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a 403 was reported as a missing object")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error does not name the status: %v", err)
	}

	// Has must not answer "no such object" either, or a caller re-stores
	// everything it already had.
	ok, err := s.Has(context.Background(), "k")
	if err == nil {
		t.Fatal("Has accepted a 403")
	}
	if ok {
		t.Error("Has reported true on a refusal")
	}
}

// The callers do not supply a deadline: the mailbox sync loop runs on a
// context derived from Background, so a request that never answers has to be
// bounded here or it wedges that mailbox forever.
func TestBrokeredClientsHaveTimeouts(t *testing.T) {
	s, err := NewBrokered("https://x", "tok")
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	if s.broker.Timeout == 0 {
		t.Error("the broker client has no timeout")
	}
	if s.transfer.Timeout == 0 {
		t.Error("the transfer client has no timeout")
	}
}

// A presigned PUT is signed for a plain request; S3 rejects the chunked
// encoding Go picks when it cannot tell the length. The reader this repo
// passes is a *bytes.Reader, so the length must survive to the request.
func TestBrokeredPutSetsContentLength(t *testing.T) {
	var gotLen int64 = -1
	var gotType string
	var gotBody []byte
	s, _ := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLen = r.ContentLength
		gotType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		if r.Method != http.MethodPut {
			t.Errorf("object store saw %s, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))

	if err := s.Put(context.Background(), "k", bytes.NewReader([]byte("hello")), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if gotLen != 5 {
		t.Errorf("Content-Length %d, want 5", gotLen)
	}
	// The signature covers the content type when one was signed, so it has to
	// go back exactly as asked for.
	if gotType != "text/plain" {
		t.Errorf("Content-Type %q, want text/plain", gotType)
	}
	if string(gotBody) != "hello" {
		t.Errorf("body %q, want hello", gotBody)
	}
}

// A reader with no length still has to produce a length, by buffering.
func TestBrokeredPutUnknownLengthIsBuffered(t *testing.T) {
	var gotLen int64 = -1
	s, _ := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLen = r.ContentLength
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	// io.MultiReader reports no length, unlike the concrete reader types.
	body := io.MultiReader(strings.NewReader("abc"), strings.NewReader("de"))
	if err := s.Put(context.Background(), "k", body, ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if gotLen != 5 {
		t.Errorf("Content-Length %d, want 5", gotLen)
	}
}

func TestBrokeredHasAndDelete(t *testing.T) {
	var methods []string
	s, _ := brokerFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	ok, err := s.Has(context.Background(), "k")
	if err != nil || !ok {
		t.Fatalf("Has = %v, %v; want true, nil", ok, err)
	}
	if err := s.Delete(context.Background(), "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodHead || methods[1] != http.MethodDelete {
		t.Errorf("object store saw %v, want [HEAD DELETE]", methods)
	}
}

// The filesystem backend cannot sign, and the broker says so with 501. That
// has to reach the caller as ErrUnsupported rather than a bare failure, since
// the fix is a different BLOB_PROVIDER and not a retry.
func TestBrokeredUnsupportedBackend(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer broker.Close()

	s, err := NewBrokered(broker.URL, "tok")
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	if _, err := s.Get(context.Background(), "k"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("got %v, want ErrUnsupported", err)
	}
}

func TestNewBrokeredRequiresBaseURLAndToken(t *testing.T) {
	if _, err := NewBrokered("", "tok"); err == nil {
		t.Error("empty baseURL accepted")
	}
	if _, err := NewBrokered("https://x", ""); err == nil {
		t.Error("empty token accepted")
	}
}

func TestPresignOpMethods(t *testing.T) {
	cases := map[PresignOp]string{
		PresignOpGet:    http.MethodGet,
		PresignOpPut:    http.MethodPut,
		PresignOpHead:   http.MethodHead,
		PresignOpDelete: http.MethodDelete,
	}
	for op, want := range cases {
		if !op.Valid() {
			t.Errorf("%s reported invalid", op)
		}
		if got := op.Method(); got != want {
			t.Errorf("%s method = %s, want %s", op, got, want)
		}
	}
	if PresignOp("list").Valid() {
		t.Error("unknown op reported valid")
	}
}
