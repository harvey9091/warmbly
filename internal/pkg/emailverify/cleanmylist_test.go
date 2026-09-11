package emailverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCleanMyListChecksAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/verify" || r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Email     string `json:"email"`
			SMTPProbe bool   `json:"smtp_probe"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email != "good@example.com" || !body.SMTPProbe {
			t.Errorf("request body = %+v, %v", body, err)
		}
		_, _ = w.Write([]byte(`{"verdict":"deliverable","score":95,"reason":"All checks passed","reason_code":"ok","checks":[{"name":"mx_records","status":"pass"}]}`))
	}))
	defer srv.Close()

	res, err := NewCleanMyList(" key ", srv.URL).Check(context.Background(), " Good@Example.com ")
	if err != nil || res.Email != "good@example.com" || res.Status != StatusValid || res.Provider != "cleanmylist" || res.Confidence != 95 || !res.HasMX || res.CheckedAt.IsZero() {
		t.Fatalf("check = %+v, %v", res, err)
	}
}

func TestCleanMyListValidatesAccountWithoutSpendingCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/jobs" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected account request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"jobs":[]}`))
	}))
	defer srv.Close()
	if balance, err := NewCleanMyList("key", srv.URL).Account(context.Background()); err != nil || balance != nil {
		t.Fatalf("account = %v, %v; balance must remain unknown", balance, err)
	}
}

func TestCleanMyListFailuresNeverRejectAnAddress(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"invalid key", 401, `{"error":{"code":"unauthorized"}}`, ErrProviderKey},
		{"unverified account", 403, `{"error":{"code":"email_not_verified"}}`, ErrProviderKey},
		{"no allowance", 402, `{"error":{"code":"out_of_credits"}}`, ErrProviderCredits},
		{"rate limited", 429, `{"error":{"code":"rate_limited"}}`, nil},
		{"server failure", 503, `unavailable`, nil},
		{"malformed response", 200, `<html>Bad gateway</html>`, nil},
		{"missing verdict", 200, `{}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client := NewCleanMyList("key", srv.URL)
			res, err := client.Check(context.Background(), "test@example.com")
			if err == nil || res.Status != StatusUnknown || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("check = %+v, %v", res, err)
			}
			if tc.status != 200 {
				if _, err := client.Account(context.Background()); err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
					t.Fatalf("account error = %v", err)
				}
			}
		})
	}
}

func TestCleanMyListMapsVerdicts(t *testing.T) {
	for _, tc := range []struct {
		verdict, reason string
		status          Status
		sub             SubStatus
	}{
		{"risky", "catch_all", StatusRisky, SubStatusCatchAll},
		{"risky", "role_account", StatusRisky, SubStatusRole},
		{"undeliverable", "disposable", StatusInvalid, SubStatusDisposable},
		{"undeliverable", "invalid_syntax", StatusInvalid, SubStatusSyntax},
		{"undeliverable", "null_mx", StatusInvalid, SubStatusNoMX},
		{"undeliverable", "mailbox_not_found", StatusInvalid, SubStatusNone},
		{"unknown", "verification_error", StatusUnknown, SubStatusNone},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"verdict":%q,"reason_code":%q,"reason":"Explanation"}`, tc.verdict, tc.reason)
			}))
			defer srv.Close()
			res, err := NewCleanMyList("key", srv.URL).Check(context.Background(), "test@example.com")
			if err != nil || res.Status != tc.status || res.SubStatus != tc.sub || res.IsCatchAll != (tc.sub == SubStatusCatchAll) || res.Reason != "cleanmylist: Explanation ("+tc.reason+")" {
				t.Fatalf("check = %+v, %v", res, err)
			}
			if v, ok := NormalizeExternal("CleanMyList", tc.verdict); !ok || v.Status != tc.status {
				t.Fatalf("imported verdict = %+v, %v", v, ok)
			}
		})
	}
}

func TestCleanMyListPreservesCatchAllAlongsideRoleReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"verdict":"risky","reason_code":"role_account","checks":[{"name":"catch_all","status":"warn","reason_code":"catch_all"}]}`))
	}))
	defer srv.Close()
	res, err := NewCleanMyList("key", srv.URL).Check(context.Background(), "info@example.com")
	if err != nil || res.SubStatus != SubStatusRole || !res.IsCatchAll {
		t.Fatalf("role on a catch-all domain = %+v, %v", res, err)
	}
}
