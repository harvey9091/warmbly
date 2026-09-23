package mailboximport

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/pkg/spreadsheet"
)

// Input is what a preview or an import reads: a file, or text pasted into the dashboard.
type Input struct {
	Filename string
	File     []byte
	Text     string
}

// table is the input as cells, before anything is decided about them.
type table struct {
	format  string
	records [][]string
}

// readInput turns a file or pasted text into rows of cells, trimmed and without blank lines.
func readInput(in Input) (*table, *errx.Error) {
	var records [][]string
	format := "paste"
	switch {
	case len(in.File) > 0:
		rows, kind, err := spreadsheet.Parse(bytes.NewReader(in.File), in.Filename, config.MailboxImportMaxRows+2)
		if err != nil {
			return nil, errx.New(errx.BadRequest, err.Error())
		}
		records, format = rows, kind
	case strings.TrimSpace(in.Text) != "":
		records = parsePaste(in.Text)
	default:
		return nil, errx.NewWithIdentifier(errx.BadRequest, "mailbox_import_empty", "Choose a file or paste a list of mailboxes.")
	}

	out := make([][]string, 0, len(records))
	for _, rec := range records {
		blank := true
		for i := range rec {
			rec[i] = strings.TrimSpace(strings.TrimPrefix(rec[i], "\ufeff"))
			if rec[i] != "" {
				blank = false
			}
		}
		if !blank {
			out = append(out, rec)
		}
	}
	if len(out) == 0 {
		return nil, errx.NewWithIdentifier(errx.BadRequest, "mailbox_import_empty", "The file has no rows.")
	}
	if len(out) > config.MailboxImportMaxRows+1 {
		return nil, errx.NewWithIdentifier(errx.BadRequest, "mailbox_import_too_large",
			"One import takes up to 5,000 mailboxes. Split the file and import the rest separately.")
	}
	return &table{format: format, records: out}, nil
}

// parsePaste reads cells copied from a spreadsheet (tabs), CSV, or one
// "address:password" pair per line, which is how vendors often hand them over.
func parsePaste(text string) [][]string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	switch {
	case strings.Contains(text, "\t"):
		out := make([][]string, 0, len(lines))
		for _, l := range lines {
			out = append(out, strings.Split(l, "\t"))
		}
		return out
	case semicolonTable(text):
		r := csv.NewReader(strings.NewReader(text))
		r.Comma, r.FieldsPerRecord, r.LazyQuotes = ';', -1, true
		if recs, err := r.ReadAll(); err == nil {
			return recs
		}
	case strings.Contains(text, ",") && !addressPairs(lines):
		r := csv.NewReader(strings.NewReader(text))
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		if recs, err := r.ReadAll(); err == nil {
			return recs
		}
	}
	out := make([][]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// The address never contains a colon or a semicolon; the password may.
		if i := strings.IndexAny(l, ":;"); i > 0 {
			out = append(out, []string{l[:i], l[i+1:]})
			continue
		}
		if f := strings.Fields(l); len(f) >= 2 {
			out = append(out, []string{f[0], strings.Join(f[1:], " ")})
			continue
		}
		out = append(out, []string{l})
	}
	return out
}

// hasHeaderRow guesses whether the first row names the columns: a header
// never holds an email address.
func hasHeaderRow(first []string) bool {
	for _, c := range first {
		if strings.Contains(c, "@") {
			return false
		}
	}
	return true
}

// semicolonTable is a pasted table separated by semicolons: three or more
// columns, under a header row or with the same count on every line, so an
// "address;password" pair whose password holds a semicolon still splits once.
func semicolonTable(text string) bool {
	if spreadsheet.Delimiter([]byte(text)) != ';' {
		return false
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return false
	}
	n := strings.Count(lines[0], ";")
	if n < 2 {
		return false
	}
	if !strings.Contains(lines[0], "@") {
		return true
	}
	for _, l := range lines[1:] {
		if strings.Count(l, ";") != n {
			return false
		}
	}
	return true
}

// addressPairs is a list of "address:password" (or ";") lines, whose passwords
// may hold commas: every line has an address, then the separator, before any comma.
func addressPairs(lines []string) bool {
	seen := false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		i := strings.IndexAny(l, ":;")
		if i <= 0 || !strings.Contains(l[:i], "@") {
			return false
		}
		if c := strings.IndexByte(l, ','); c >= 0 && c < i {
			return false
		}
		seen = true
	}
	return seen
}
