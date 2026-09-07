package models

import "time"

type Mailbox struct {
	Name          string   `json:"name"`
	Attrs         []string `json:"attributes"`
	UIDValidity   uint32   `json:"uid_validity"`
	HighestModSeq uint64   `json:"highestmodseq"`
	// UIDNext is the folder's next UID as last seen. It is the incremental
	// cursor on a server without CONDSTORE, where HighestModSeq stays 0.
	UIDNext uint32 `json:"uid_next"`
	// Delim is the hierarchy delimiter this server reported for the folder
	// ("/" on Gmail, "." on many Dovecots). Empty when the server reported
	// none, where the leaf is guessed instead.
	Delim string `json:"delim,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
}
