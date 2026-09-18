package imap

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

// The stored form has to be one net/mail parses, the same one the Gmail and
// Graph syncs write; "Name (addr)" was refused by every address check on the
// backend, which is how replies into IMAP mailboxes stopped counting.
func TestGetAddressNameIsTheStandardForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr imap.Address
		want string
	}{
		{"name and address", imap.Address{Name: "M Kannan", Mailbox: "kannan", Host: "eml.example.com"}, "M Kannan <kannan@eml.example.com>"},
		{"bare", imap.Address{Mailbox: "kannan", Host: "eml.example.com"}, "kannan@eml.example.com"},
		{"encoded name", imap.Address{Name: "=?utf-8?q?Ana_Rodr=C3=ADguez?=", Mailbox: "ana", Host: "example.com"}, "Ana Rodríguez <ana@example.com>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := GetAddressName(tc.addr); got != tc.want {
				t.Fatalf("GetAddressName = %q, want %q", got, tc.want)
			}
		})
	}
}
