package worker

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// A warmup action addresses a message by UID, and a UID only means something
// inside one folder and one generation of that folder's UIDs. Resolving it by
// UIDVALIDITY alone was both: wrong on a server that gives several folders the
// same number, where the action landed in whichever folder matched first, and
// unable to tell a reissued number from the original.
func TestLookupWarmupSourceFolder(t *testing.T) {
	boxes := []*models.Mailbox{
		{Name: "INBOX", UIDValidity: 7},
		// A folder tree created in the same second on a server that stamps
		// UIDVALIDITY with the creation time.
		{Name: "Clients/Acme", UIDValidity: 42},
		{Name: "Clients/Globex", UIDValidity: 42},
	}

	for _, tc := range []struct {
		name   string
		action models.WarmupEmailAction
		want   string
	}{
		{
			"the named folder wins over another with the same UIDVALIDITY",
			models.WarmupEmailAction{MailboxFolder: "Clients/Globex", MailboxUIDValidity: 42},
			"Clients/Globex",
		},
		{
			"a folder whose UIDs were reissued is not acted on",
			models.WarmupEmailAction{MailboxFolder: "INBOX", MailboxUIDValidity: 6},
			"",
		},
		{
			"a folder that is gone is not acted on",
			models.WarmupEmailAction{MailboxFolder: "Clients/Initech", MailboxUIDValidity: 42},
			"",
		},
		// An action published before the folder name was carried has only the
		// number, which is the old behaviour and stays the fallback.
		{
			"no name falls back to the UIDVALIDITY",
			models.WarmupEmailAction{MailboxUIDValidity: 7},
			"INBOX",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := lookupWarmupSourceFolder(boxes, tc.action)
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("resolved %q, want nothing to act on", got.Name)
			case tc.want != "" && got == nil:
				t.Fatalf("resolved nothing, want %q", tc.want)
			case tc.want != "" && got.Name != tc.want:
				t.Fatalf("resolved %q, want %q", got.Name, tc.want)
			}
		})
	}
}
