package imap

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// Everything downstream identifies a folder by its UIDVALIDITY, and the
// database keys the folder row on it, but RFC 3501 only promises UIDs are
// stable within one folder: Dovecot and others derive UIDVALIDITY from the
// creation time, so a folder tree created in one second shares one. Two
// folders under a single id would advance each other's cursor and delete each
// other's row, so only one is followed.
func TestRankFoldersPutsInboxAndSpecialFoldersFirst(t *testing.T) {
	all := []models.Mailbox{
		{Name: "Projects"},
		{Name: "Archive"},
		{Name: "INBOX"},
		{Name: "Notes"},
		{Name: "Sent", Attrs: []string{"\\Sent"}},
	}
	kept, overflow := rankFolders(all, 3)
	if overflow != 2 {
		t.Fatalf("overflow = %d, want 2", overflow)
	}
	if kept[0].Name != "INBOX" {
		t.Fatalf("kept[0] = %q, want INBOX to survive any cap", kept[0].Name)
	}
	for _, want := range []string{"Sent", "Archive"} {
		found := false
		for _, b := range kept {
			if b.Name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was cut in favour of a plain user folder", want)
		}
	}
}

// A listing that fits keeps every folder, and is still ranked: the pass walks
// folders in this order and can run out of budget partway, so the inbox and
// the special folders should be the ones that got through.
func TestRankFoldersKeepsEverythingUnderTheCap(t *testing.T) {
	all := []models.Mailbox{{Name: "Work"}, {Name: "INBOX"}, {Name: "Sent"}}
	kept, overflow := rankFolders(all, 10)
	if overflow != 0 {
		t.Fatalf("overflow = %d with everything under the cap", overflow)
	}
	if len(kept) != 3 {
		t.Fatalf("kept %d folders, want all 3", len(kept))
	}
	if kept[0].Name != "INBOX" || kept[1].Name != "Sent" || kept[2].Name != "Work" {
		t.Fatalf("order = %+v, want the inbox then the special folder then the user folder", kept)
	}
}

// A container the server lists only to show hierarchy cannot be SELECTed, so
// following it would fail every pass.
func TestSelectableFolder(t *testing.T) {
	for _, tc := range []struct {
		attrs []string
		want  bool
	}{
		{[]string{"\\HasChildren"}, true},
		{nil, true},
		{[]string{"\\Noselect"}, false},
		{[]string{"\\NonExistent"}, false},
		{[]string{"\\NoSelect", "\\HasChildren"}, false},
	} {
		if got := selectableFolder(tc.attrs); got != tc.want {
			t.Errorf("selectableFolder(%v) = %v, want %v", tc.attrs, got, tc.want)
		}
	}
}

// A server that publishes no special-use attributes reports its folders in
// the mailbox owner's language, and an unmatched Sent folder means the
// customer's sent mail shows up in their inbox instead.
func TestCanonicalFolderLocalizedNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"Gesendete Elemente", models.FolderSent},
		{"Éléments envoyés", models.FolderSent},
		{"Elementos enviados", models.FolderSent},
		{"Elküldött elemek", models.FolderSent},
		{"INBOX.Papierkorb", models.FolderTrash},
		{"Corbeille", models.FolderTrash},
		{"Posta indesiderata", models.FolderSpam},
		{"Skräppost", models.FolderSpam},
		{"Entwürfe", models.FolderDrafts},
		{"Archiwum", models.FolderArchive},
		// A user folder that merely mentions a role stays where it is.
		{"Spam reports", models.FolderInbox},
		{"Sent to legal", models.FolderInbox},
		{"Clients/Acme", models.FolderInbox},
	} {
		if got := CanonicalFolder(models.Mailbox{Name: tc.name}); got != tc.want {
			t.Errorf("CanonicalFolder(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The spam guard decides whether a warmup action may MOVE a message out of a
// folder, so a folder that merely mentions spam must not qualify.
func TestIsSpamMailboxName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"Spam", true},
		{"Junk", true},
		{"[Gmail]/Spam", true},
		{"INBOX.Junk E-Mail", true},
		{"Courrier indésirable", true},
		{"Spam reports", false},
		{"Junk drawer", false},
		{"INBOX", false},
	} {
		if got := IsSpamMailboxName(tc.name); got != tc.want {
			t.Errorf("IsSpamMailboxName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Gmail's label views duplicate every message under another UID; following
// one would re-file known mail as archive and swap the (mailbox, uid) pair
// the warmup actions address.
func TestIsVirtualFolder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs []string
		want  bool
	}{
		{"[Gmail]/All Mail", []string{"\\All"}, true},
		{"[Gmail]/All Mail", []string{"\\HasNoChildren"}, true},
		{"[Google Mail]/Important", nil, true},
		{"[Gmail]/Starred", nil, true},
		{"[Gmail]/Sent Mail", []string{"\\Sent"}, false},
		{"[Gmail]/Bin", []string{"\\Trash"}, false},
		{"Important", nil, false},
		{"All Mail", nil, false},
		{"INBOX", nil, false},
	} {
		if got := IsVirtualFolder(models.Mailbox{Name: tc.name, Attrs: tc.attrs}); got != tc.want {
			t.Errorf("IsVirtualFolder(%q, %v) = %v, want %v", tc.name, tc.attrs, got, tc.want)
		}
	}
}

// Two folders sharing a UIDVALIDITY is a server doing something RFC 3501 does
// not forbid but everything downstream assumes away: the folder row is keyed
// on it. Following both would advance each other's cursor and delete each
// other's row, so one is dropped and reported.
func TestDedupeByUIDValidity(t *testing.T) {
	kept, conflicts := dedupeByUIDValidity([]models.Mailbox{
		{Name: "INBOX", UIDValidity: 100},
		{Name: "Sent", UIDValidity: 200},
		// Created in the same second as Sent on a server that stamps
		// UIDVALIDITY with the creation time.
		{Name: "Projects", UIDValidity: 200},
		{Name: "Notes", UIDValidity: 300},
	})
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want 1", conflicts)
	}
	if len(kept) != 3 {
		t.Fatalf("kept %d folders, want 3", len(kept))
	}
	// The ranked order puts the special folder first, so Sent is the one that
	// survives and the plain user folder is the one dropped.
	if kept[1].Name != "Sent" {
		t.Errorf("kept[1] = %q, want Sent to win the collision", kept[1].Name)
	}
	seen := map[uint32]bool{}
	for _, b := range kept {
		if seen[b.UIDValidity] {
			t.Fatalf("UIDVALIDITY %d survived twice", b.UIDValidity)
		}
		seen[b.UIDValidity] = true
	}
}

// The common case must not allocate a conflict or reorder anything.
func TestDedupeByUIDValidityLeavesADistinctListingAlone(t *testing.T) {
	in := []models.Mailbox{{Name: "INBOX", UIDValidity: 1}, {Name: "Sent", UIDValidity: 2}}
	kept, conflicts := dedupeByUIDValidity(in)
	if conflicts != 0 || len(kept) != 2 || kept[0].Name != "INBOX" || kept[1].Name != "Sent" {
		t.Fatalf("a listing with distinct ids was changed: %+v, conflicts %d", kept, conflicts)
	}
}

// A server picks its own hierarchy delimiter and reports it on every LIST
// reply. Splitting on "." and "/" alone leaves a folder under any other
// separator unclassified, so the customer's sent mail shows up in the inbox
// scope; a server with no hierarchy at all reports NIL, which must not be
// read as a NUL separator.
func TestCanonicalFolderUsesTheServerDelimiter(t *testing.T) {
	for _, tc := range []struct {
		box  models.Mailbox
		want string
	}{
		{models.Mailbox{Name: "Parent|Sent Items", Delim: "|"}, models.FolderSent},
		{models.Mailbox{Name: `Parent\Trash`, Delim: `\`}, models.FolderTrash},
		{models.Mailbox{Name: "INBOX.Sent", Delim: "."}, models.FolderSent},
		{models.Mailbox{Name: "INBOX/Sent", Delim: "/"}, models.FolderSent},
		// No delimiter reported: fall back to the "." and "/" guess.
		{models.Mailbox{Name: "INBOX.Sent"}, models.FolderSent},
		{models.Mailbox{Name: "Sent"}, models.FolderSent},
		// A dot in the name is part of the name on a "/" server.
		{models.Mailbox{Name: "Q1.Reports", Delim: "/"}, models.FolderInbox},
	} {
		if got := CanonicalFolder(tc.box); got != tc.want {
			t.Errorf("CanonicalFolder(%q delim %q) = %q, want %q", tc.box.Name, tc.box.Delim, got, tc.want)
		}
	}
}

// The delimiter LIST reports is a rune, and a server with no hierarchy sends
// NIL, which arrives as 0. Converting that straight to a string yields a NUL
// byte, which matches nothing and hides the fallback.
func TestDelimString(t *testing.T) {
	if got := delimString(0); got != "" {
		t.Errorf("delimString(NIL) = %q, want an empty string", got)
	}
	if got := delimString('/'); got != "/" {
		t.Errorf("delimString('/') = %q", got)
	}
}
