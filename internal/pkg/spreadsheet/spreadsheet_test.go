package spreadsheet

import (
	"strings"
	"testing"
)

func TestDelimiterFollowsTheHeader(t *testing.T) {
	cases := map[string]rune{
		"email,password\na@x.test,p":                   ',',
		"email;password;name\na@x.test;p;A":            ';',
		"\ufeffemail;password\na@x.test;p":             ';',
		"email\tpassword\na@x.test\tp":                 '\t',
		`"Smith, Ann";email;password` + "\n" + "x;y;z": ';',
		"email\na@x.test":                              ',',
		"\n\nemail;password\na;b":                      ';',
	}
	for in, want := range cases {
		if got := Delimiter([]byte(in)); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestParseReadsASemicolonFile(t *testing.T) {
	rows, kind, err := Parse(strings.NewReader("email;password\na@x.test;secret, with comma\n"), "boxes.csv", 10)
	if err != nil || kind != "csv" || len(rows) != 2 || len(rows[1]) != 2 || rows[1][1] != "secret, with comma" {
		t.Fatalf("rows = %q, %s, %v", rows, kind, err)
	}
}
