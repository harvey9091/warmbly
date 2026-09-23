package msgraph

import "testing"

func TestRootActsForTheSignedInUserOrTheNamedOne(t *testing.T) {
	if got := (&Client{}).root(); got != graphBase+"/me" {
		t.Fatalf("delegated root = %q", got)
	}
	if got := (&Client{User: "8f2a/..%2f"}).root(); got != graphBase+"/users/8f2a%2F..%252f" {
		t.Fatalf("application root = %q, want the id escaped into one path segment", got)
	}
}
