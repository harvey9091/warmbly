package imap

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/warmbly/warmbly/internal/pkg/mailhdr"
)

// GetAddressName renders an envelope address the way the Gmail and Graph
// syncs do: "Name <addr>", or the bare address when there is no name. It used
// to be "Name (addr)", a form net/mail cannot parse, so every backend check
// that compared a stored address against a mailbox or a contact failed for
// IMAP mailboxes alone. mailhdr.Bare still reads the old form for rows and
// workers that predate this.
func GetAddressName(address imap.Address) string {
	addr := address.Addr()
	name := mailhdr.DecodeWords(address.Name)
	if name == "" {
		return addr
	}
	return fmt.Sprintf("%s <%s>", name, addr)
}

func GetAddressNames(addresses []imap.Address) []string {
	var addrs []string = make([]string, len(addresses))
	for i, addr := range addresses {
		addrs[i] = GetAddressName(addr)
	}
	return addrs
}
