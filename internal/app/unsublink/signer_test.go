package unsublink

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRoundTrip(t *testing.T) {
	s := New("secret", "https://api.example.com/")
	org, camp, contact := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	u := s.URL(org, camp, contact, now)
	if !strings.HasPrefix(u, "https://api.example.com/unsubscribe/") {
		t.Fatalf("unexpected url %q", u)
	}
	tok := strings.TrimPrefix(u, "https://api.example.com/unsubscribe/")
	c, err := s.Verify(tok, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.OrgID != org || c.CampaignID != camp || c.ContactID != contact {
		t.Fatalf("claims mismatch: %+v", c)
	}
	if !c.ExpiresAt.Equal(now.Add(Validity)) {
		t.Fatalf("expiry %v", c.ExpiresAt)
	}
	if _, err := s.Verify(tok, now.Add(Validity)); err != ErrExpired {
		t.Fatalf("want expired, got %v", err)
	}
}

func TestTamperAndWrongKey(t *testing.T) {
	s := New("secret", "https://api.example.com")
	tok := s.Token(uuid.New(), uuid.New(), uuid.New(), time.Now())

	if _, err := New("other", "https://api.example.com").Verify(tok, time.Now()); err != ErrInvalid {
		t.Fatalf("wrong key: want invalid, got %v", err)
	}
	flipped := []byte(tok)
	flipped[3] ^= 1
	if _, err := s.Verify(string(flipped), time.Now()); err != ErrInvalid {
		t.Fatalf("tampered: want invalid, got %v", err)
	}
	if _, err := s.Verify("", time.Now()); err != ErrInvalid {
		t.Fatalf("empty: want invalid, got %v", err)
	}
}

func TestDisabledWithoutBase(t *testing.T) {
	s := New("secret", "")
	if s.Enabled() {
		t.Fatal("expected disabled")
	}
	if s.URL(uuid.New(), uuid.New(), uuid.New(), time.Now()) != "" {
		t.Fatal("expected empty url")
	}
}
