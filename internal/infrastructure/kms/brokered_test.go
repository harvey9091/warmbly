package kms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrokeredGetDecryptedKey(t *testing.T) {
	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i)
	}
	var gotAuth, gotPath, gotCiphertext string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		var in decryptRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotCiphertext = in.EncryptedDataKey
		_ = json.NewEncoder(w).Encode(decryptResponse{
			DataKey: base64.StdEncoding.EncodeToString(want),
		})
	}))
	defer srv.Close()

	p, err := NewBrokered(srv.URL, "tok")
	if err != nil {
		t.Fatalf("NewBrokered: %v", err)
	}
	got, err := p.GetDecryptedKey(context.Background(), "sealed")
	if err != nil {
		t.Fatalf("GetDecryptedKey: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("key mismatch")
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header %q", gotAuth)
	}
	if gotPath != "/api/v1/internal/dek/decrypt" {
		t.Errorf("path %q", gotPath)
	}
	if gotCiphertext != "sealed" {
		t.Errorf("ciphertext %q", gotCiphertext)
	}
}

// Everything in this system is AES-256. A short key means the control plane
// answered with something that is not a DEK, and sealing data with it would be
// worse than failing.
func TestBrokeredRejectsShortKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(decryptResponse{
			DataKey: base64.StdEncoding.EncodeToString([]byte("too short")),
		})
	}))
	defer srv.Close()

	p, _ := NewBrokered(srv.URL, "tok")
	if _, err := p.GetDecryptedKey(context.Background(), "sealed"); err == nil {
		t.Fatal("a 9-byte data key was accepted")
	}
}

// A node must never mint an organization's key: the control plane does that
// when the org's first secret is stored, and a second minter races it.
func TestBrokeredCannotGenerate(t *testing.T) {
	p, _ := NewBrokered("https://x", "tok")
	if _, _, err := p.GenerateDataKey(context.Background()); err == nil {
		t.Fatal("a node was allowed to mint a data key")
	}
}

func TestBrokeredErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p, _ := NewBrokered(srv.URL, "tok")
	_, err := p.GetDecryptedKey(context.Background(), "sealed")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("got %v, want an error naming the status", err)
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
