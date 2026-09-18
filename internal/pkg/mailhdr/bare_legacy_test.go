package mailhdr

import "testing"

// The IMAP sync stored "Name (addr)" until v0.4.25. Old rows and old workers
// still hand that form to every address comparison, so Bare reads it.
func TestBareReadsEveryStoredForm(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Ana <a@b.com>", "a@b.com"},
		{"a@b.com", "a@b.com"},
		{"M K (mkmsc74@gmail.com)", "mkmsc74@gmail.com"},
		{" (kannan@eml.example.com)", "kannan@eml.example.com"},
		{"Kannan (M) (kannan@eml.example.com)", "kannan@eml.example.com"},
		{"Ana (Sales) <a@b.com>", "a@b.com"},
		{"Nobody (no address here)", "Nobody (no address here)"},
		{"", ""},
	} {
		if got := Bare(tc.in); got != tc.want {
			t.Errorf("Bare(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
