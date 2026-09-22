package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// aclRefusal is what S3 answers to any ACL on a bucket whose Object Ownership
// is BucketOwnerEnforced, the default for every bucket created since April
// 2023.
const aclRefusal = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<Error><Code>AccessControlListNotSupported</Code>` +
	`<Message>The bucket does not allow ACLs</Message></Error>`

// testClient points a real S3 client at a local handler.
func testClient(t *testing.T, bucket string, h http.Handler) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Setenv("AWS_ENDPOINT_URL", srv.URL)

	c, err := NewClient(context.Background(), aws.Config{
		Region:       "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("id", "secret", ""),
		BaseEndpoint: aws.String(srv.URL),
	}, bucket)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv.Close
}

func TestPutPublicWritesTheObjectWhenTheBucketRefusesACLs(t *testing.T) {
	var puts, acls atomic.Int32

	// BucketOwnerEnforced refuses an ACL however it arrives: as the ?acl
	// sub-resource, or as the canned header on the write itself. Refusing both
	// is what makes this test fail if the ACL is ever folded back into the
	// PutObject, which is the shape that broke in production.
	c, closeSrv := testClient(t, "b", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("acl") || r.Header.Get("x-amz-acl") != "" {
			acls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, aclRefusal)
			return
		}
		puts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer closeSrv()
	c.PublicBaseURL = "https://api.example.com/public"

	// The ACL used to ride along on the PutObject, so this refusal failed the
	// whole write and the avatar was never stored.
	url, err := c.PutPublic(context.Background(), "avatars/users/a.png", strings.NewReader("x"), "image/png")
	if err != nil {
		t.Fatalf("PutPublic: %v", err)
	}
	if want := "https://api.example.com/public/avatars/users/a.png"; url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}
	if puts.Load() != 1 {
		t.Fatalf("object writes = %d, want 1", puts.Load())
	}

	// And the refusal is remembered, so the second upload does not ask again.
	if _, err := c.PutPublic(context.Background(), "avatars/users/b.png", strings.NewReader("x"), "image/png"); err != nil {
		t.Fatalf("second PutPublic: %v", err)
	}
	if acls.Load() != 1 {
		t.Fatalf("ACL attempts = %d, want 1", acls.Load())
	}
}

func TestPutPublicStillGrantsPublicReadWhereItIsTaken(t *testing.T) {
	var acls atomic.Int32

	c, closeSrv := testClient(t, "b", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("acl") {
			acls.Add(1)
			if got := r.Header.Get("x-amz-acl"); got != "public-read" {
				t.Errorf("x-amz-acl = %q, want public-read", got)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer closeSrv()

	// No public base URL: the object is fetched straight from the store, so a
	// bucket that still publishes through ACLs has to keep getting one.
	for i := 0; i < 2; i++ {
		if _, err := c.PutPublic(context.Background(), "avatars/users/a.png", strings.NewReader("x"), "image/png"); err != nil {
			t.Fatalf("PutPublic: %v", err)
		}
	}
	if acls.Load() != 2 {
		t.Fatalf("ACL attempts = %d, want 2", acls.Load())
	}
}

func TestPutPublicFailsWhenTheObjectItselfIsRefused(t *testing.T) {
	c, closeSrv := testClient(t, "b", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>nope</Message></Error>`)
	}))
	defer closeSrv()

	// Only the ACL is best-effort. A write that did not land is still an error.
	if _, err := c.PutPublic(context.Background(), "avatars/users/a.png", strings.NewReader("x"), "image/png"); err == nil {
		t.Fatal("PutPublic succeeded on a refused write")
	}
}
