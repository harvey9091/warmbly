package paging

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMergedCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 9, 11, 42, 0, 123456000, time.FixedZone("CEST", 2*3600))
	id := uuid.New()
	tok := EncodeMerged(at, 7, id)
	if tok == nil {
		t.Fatal("want a token for a real position")
	}
	gotAt, gotSource, gotID, xerr := DecodeMergedCursor(*tok)
	if xerr != nil {
		t.Fatalf("decode: %v", xerr)
	}
	if !gotAt.Equal(at) || gotSource != 7 || gotID != id {
		t.Fatalf("round trip lost data: %v %d %s", gotAt, gotSource, gotID)
	}
	if EncodeMerged(at, 7, uuid.Nil) != nil {
		t.Fatal("the zero id means no next page and must encode as nil")
	}
}

func TestMergedCursorRejectsMalformedTokens(t *testing.T) {
	if _, _, _, xerr := DecodeMergedCursor(""); xerr != nil {
		t.Fatalf("an empty token is the first page, got %v", xerr)
	}
	for _, tok := range []string{
		"2026-06-09T11:42:00Z",                            // a bare timestamp is not a cursor
		"t1_" + (*EncodeTime(time.Now(), uuid.New()))[3:], // wrong version
		"m1_!!!", // not base64
		"m1_" + (*EncodeMerged(time.Now(), 1, uuid.New()))[3:] + "x", // trailing garbage
	} {
		if _, _, _, xerr := DecodeMergedCursor(tok); xerr == nil {
			t.Fatalf("%q must be rejected", tok)
		}
	}
}

func TestSortCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	val := "2026-06-09 11:42:00.123456"
	tok := EncodeSort("created_at:desc", &val, id)
	if tok == nil {
		t.Fatal("want a token for a real position")
	}
	got, xerr := DecodeSortCursor(*tok)
	if xerr != nil {
		t.Fatalf("decode: %v", xerr)
	}
	if got.Sort != "created_at:desc" || got.ID != id || got.Value == nil || *got.Value != val {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if EncodeSort("created_at:desc", &val, uuid.Nil) != nil {
		t.Fatal("the zero id means no next page and must encode as nil")
	}
}

// A NULL sort value is a real keyset position (the NULL block of a nullable
// column), and must survive the round trip as NULL rather than as "".
func TestSortCursorCarriesANullValue(t *testing.T) {
	id := uuid.New()
	got, xerr := DecodeSortCursor(*EncodeSort("first_name:asc", nil, id))
	if xerr != nil {
		t.Fatalf("decode: %v", xerr)
	}
	if got.Value != nil || got.ID != id || got.Sort != "first_name:asc" {
		t.Fatalf("want a NULL boundary, got %+v", got)
	}
	empty := ""
	got, xerr = DecodeSortCursor(*EncodeSort("first_name:asc", &empty, id))
	if xerr != nil {
		t.Fatalf("decode empty: %v", xerr)
	}
	if got.Value == nil || *got.Value != "" {
		t.Fatalf("an empty string is not NULL, got %+v", got)
	}
}

// Values are user data: separators in them must not shift the fields.
func TestSortCursorKeepsSeparatorsInTheValue(t *testing.T) {
	id := uuid.New()
	val := "a|b|c"
	got, xerr := DecodeSortCursor(*EncodeSort("last_name:asc", &val, id))
	if xerr != nil {
		t.Fatalf("decode: %v", xerr)
	}
	if got.Value == nil || *got.Value != val {
		t.Fatalf("value = %v, want %q", got.Value, val)
	}
}

func TestSortCursorRejectsMalformedTokens(t *testing.T) {
	if c, xerr := DecodeSortCursor(""); xerr != nil || c != nil {
		t.Fatalf("an empty token is the first page, got %v %v", c, xerr)
	}
	for _, tok := range []string{
		"s1_",                                 // no payload
		uuid.New().String(),                   // a bare id is not a cursor
		"c1_" + (*EncodeUUID(uuid.New()))[3:], // the id-only format
		"s1_!!!",                              // not base64
		"s1_" + base64Raw("1"+uuid.New().String()),                // no sort key
		"s1_" + base64Raw("2"+uuid.New().String()+"|email:asc|x"), // unknown flag
		"s1_" + base64Raw("1nope|email:asc|x"),                    // not an id
	} {
		if _, xerr := DecodeSortCursor(tok); xerr == nil {
			t.Fatalf("%q must be rejected", tok)
		}
	}
}

func base64Raw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
