package emailverify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMillionVerifierMapsResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api") != "key" {
			http.Error(w, `{"error":"api key not found"}`, http.StatusOK)
			return
		}
		switch r.URL.Path {
		case "/api/v3/credits":
			_, _ = w.Write([]byte(`{"credits": 42}`))
		case "/api/v3/":
			switch r.URL.Query().Get("email") {
			case "good@x.com":
				_, _ = w.Write([]byte(`{"email":"good@x.com","quality":"good","result":"ok","resultcode":1,"subresult":"","free":false,"role":false,"error":"","credits":41}`))
			case "info@x.com":
				_, _ = w.Write([]byte(`{"email":"info@x.com","quality":"good","result":"ok","resultcode":1,"subresult":"","free":false,"role":true,"error":"","credits":40}`))
			case "gone@x.com":
				_, _ = w.Write([]byte(`{"email":"gone@x.com","quality":"bad","result":"invalid","resultcode":6,"subresult":"user_unknown","free":false,"role":false,"error":"","credits":39}`))
			default:
				_, _ = w.Write([]byte(`{"error":"insufficient credits"}`))
			}
		}
	}))
	defer srv.Close()

	mv := NewMillionVerifier("key", srv.URL)
	if n, err := mv.Credits(context.Background()); err != nil || n != 42 {
		t.Fatalf("credits = %d, %v", n, err)
	}
	res, err := mv.Check(context.Background(), "good@x.com")
	if err != nil || res.Status != StatusValid || res.Provider != ProviderMillionVerifier {
		t.Fatalf("good = %+v, %v", res, err)
	}
	res, _ = mv.Check(context.Background(), "info@x.com")
	if res.Status != StatusRisky || res.SubStatus != SubStatusRole {
		t.Fatalf("role = %+v", res)
	}
	res, _ = mv.Check(context.Background(), "gone@x.com")
	if res.Status != StatusInvalid || res.Reason != "millionverifier: invalid (user_unknown)" {
		t.Fatalf("gone = %+v", res)
	}
	res, err = mv.Check(context.Background(), "broke@x.com")
	if !errors.Is(err, ErrProviderCredits) || res.Status != StatusUnknown {
		t.Fatalf("no credits = %+v, %v", res, err)
	}

	bad := NewMillionVerifier("wrong", srv.URL)
	if _, err := bad.Credits(context.Background()); !errors.Is(err, ErrProviderKey) {
		t.Fatalf("bad key = %v", err)
	}
}
