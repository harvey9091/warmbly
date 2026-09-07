package wmail

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// imapCanonicalFolder decides which sidebar folder a message lands in, so a
// wrong answer here silently files mail under the wrong scope.
func TestImapCanonicalFolder(t *testing.T) {
	for _, tc := range []struct {
		name string
		box  models.Mailbox
		want string
	}{
		// Special-use attributes are authoritative.
		{"sent by attribute", models.Mailbox{Name: "Whatever", Attrs: []string{"\\Sent"}}, models.FolderSent},
		{"drafts by attribute", models.Mailbox{Name: "Whatever", Attrs: []string{"\\Drafts"}}, models.FolderDrafts},
		{"junk by attribute", models.Mailbox{Name: "Whatever", Attrs: []string{"\\Junk"}}, models.FolderSpam},
		{"trash by attribute", models.Mailbox{Name: "Whatever", Attrs: []string{"\\Trash"}}, models.FolderTrash},
		{"archive by attribute", models.Mailbox{Name: "Whatever", Attrs: []string{"\\Archive"}}, models.FolderArchive},
		{"all mail is archive", models.Mailbox{Name: "[Gmail]/All Mail", Attrs: []string{"\\All"}}, models.FolderArchive},
		// Name fallback for servers that advertise no special-use.
		{"sent by name", models.Mailbox{Name: "Sent Items"}, models.FolderSent},
		{"sent by nested name", models.Mailbox{Name: "INBOX.Sent"}, models.FolderSent},
		{"spam by name", models.Mailbox{Name: "Junk E-Mail"}, models.FolderSpam},
		{"trash by name", models.Mailbox{Name: "Deleted Items"}, models.FolderTrash},
		{"gmail bin is trash", models.Mailbox{Name: "[Gmail]/Bin"}, models.FolderTrash},
		{"drafts by name", models.Mailbox{Name: "Drafts"}, models.FolderDrafts},
		// Anything unrecognised stays visible rather than vanishing into a
		// scope the user never opens.
		{"inbox", models.Mailbox{Name: "INBOX"}, models.FolderInbox},
		{"user folder", models.Mailbox{Name: "INBOX.Clients.Acme"}, models.FolderInbox},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imapCanonicalFolder(&tc.box); got != tc.want {
				t.Fatalf("imapCanonicalFolder(%+v) = %q, want %q", tc.box, got, tc.want)
			}
		})
	}
}

// Gmail's label views duplicate every message under another UID; a pass that
// followed them would re-file INBOX mail as archive and swap the (mailbox,
// uid) pair warmup actions address. Sent must NOT be virtual: it is the folder
// the "*" listing exists to reach.
func TestImapVirtualFolder(t *testing.T) {
	for _, tc := range []struct {
		box  models.Mailbox
		want bool
	}{
		{models.Mailbox{Name: "[Gmail]/All Mail", Attrs: []string{"\\All", "\\HasNoChildren"}}, true},
		{models.Mailbox{Name: "[Gmail]/Starred", Attrs: []string{"\\Flagged"}}, true},
		{models.Mailbox{Name: "[Gmail]/Important", Attrs: []string{"\\Important"}}, true},
		// A plain LIST (no SPECIAL-USE) carries only \HasNoChildren; the name
		// fallback applies inside Gmail's namespace only.
		{models.Mailbox{Name: "[Gmail]/All Mail", Attrs: []string{"\\HasNoChildren"}}, true},
		{models.Mailbox{Name: "[Gmail]/Starred"}, true},
		{models.Mailbox{Name: "[Google Mail]/Important"}, true},
		// Ordinary IMAP folders that happen to share the names are real.
		{models.Mailbox{Name: "Important"}, false},
		{models.Mailbox{Name: "INBOX.Starred"}, false},
		{models.Mailbox{Name: "All Mail"}, false},
		{models.Mailbox{Name: "[Gmail]/Sent Mail", Attrs: []string{"\\Sent"}}, false},
		{models.Mailbox{Name: "[Gmail]/Bin", Attrs: []string{"\\Trash"}}, false},
		{models.Mailbox{Name: "INBOX"}, false},
	} {
		if got := imapVirtualFolder(&tc.box); got != tc.want {
			t.Errorf("imapVirtualFolder(%q) = %v, want %v", tc.box.Name, got, tc.want)
		}
	}
}

// A virtual folder is never baselined, and one a previous build did baseline
// is retired through the deletion sweep so its cursor leaves the store.
func TestImapSyncSkipsVirtualFolders(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100},
		{Name: "[Gmail]/All Mail", UIDValidity: 9, HighestModSeq: 100, Attrs: []string{"\\All"}},
		{Name: "[Gmail]/Starred", UIDValidity: 11, HighestModSeq: 100, Attrs: []string{"\\Flagged"}},
	}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10}, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes,
		&models.Mailbox{Name: "[Gmail]/Starred", UIDValidity: 11, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	retired := false
	for _, e := range *events {
		switch e.eventType {
		case models.JobEventTypeMailboxUpdate:
			if got := e.body.(*models.JobEventMailboxUpdate).Data.UIDValidity; got == 9 {
				t.Fatal("All Mail was baselined; virtual folders must be skipped")
			}
		case models.JobEventTypeMailboxDelete:
			if e.body.(*models.JobEventMailboxDelete).UIDValidity == 11 {
				retired = true
			}
		}
	}
	if !retired {
		t.Error("the stale Starred cursor was not retired with a MAILBOX_DELETE")
	}
	if len(w.SmtpImapData.Mailboxes) != 1 {
		t.Fatalf("tracked %d folders, want just INBOX", len(w.SmtpImapData.Mailboxes))
	}
}

// Drafts is imported now that the folder sidebar gives it a destination; the
// rest of the eligibility matrix lives in TestImapBackfillEligible.
func TestImapBackfillEligible_DraftsByName(t *testing.T) {
	for _, name := range []string{"Drafts", "Draft", "INBOX.Drafts"} {
		if !imapBackfillEligible(&models.Mailbox{Name: name}) {
			t.Fatalf("imapBackfillEligible(%q) = false, want true", name)
		}
	}
}
