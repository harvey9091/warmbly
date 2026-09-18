package imap

import (
	"context"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// The two ways IMAP warmup filing was silently leaving mail in the inbox in
// production (#586). Each test was run against the bug first and watched fail.

// A server that lists the folder under a different spelling than the one we
// created it with ("/Warmbly" for "Warmbly") made LIST miss it and CREATE
// answer ALREADYEXISTS, which the client treated as a failed move. Every
// warmup arrival on ten mailboxes stayed in the inbox as a result.
func TestCreateMailboxTreatsAlreadyExistsAsCreated(t *testing.T) {
	c := testServer(t, nil)
	c.mu.Lock()
	defer c.mu.Unlock()
	// The exported methods dial lazily under mu; this reaches below them.
	if merr := c.ensureConnected(); merr != nil {
		t.Fatalf("connect: %v", merr)
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()

	if err := c.createMailboxLocked("Warmbly"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// The memory server answers the second CREATE with NO [ALREADYEXISTS],
	// which is the exact response the production host gave.
	if err := c.createMailboxLocked("Warmbly"); err != nil {
		t.Fatalf("a folder that already exists is the outcome we wanted, not an error: %v", err)
	}
}

func TestSameMailboxIgnoresCaseAndALeadingDelimiter(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"/Warmbly", "Warmbly", true},
		{"warmbly", "Warmbly", true},
		{".Warmbly", "Warmbly", true},
		{"INBOX.Warmbly", "Warmbly", false},
		{"Archive/Warmbly", "Warmbly", false},
		{"Warmbly", "Warmbly2", false},
	} {
		if got := sameMailbox(tc.a, tc.b); got != tc.same {
			t.Errorf("sameMailbox(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.same)
		}
	}
}

// On a Dovecot that keeps user folders under "INBOX." the filing move
// qualified its destination but the engagement leg selected the bare name the
// worker handed back, and the server refused it as a nonexistent namespace.
func TestMarkAsReadQualifiesTheFolderName(t *testing.T) {
	c, wire := recordingServer(t, nil, "INBOX.Warmbly")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	prefix := "INBOX."
	c.mu.Lock()
	c.nsPrefix = &prefix
	c.mu.Unlock()
	appendMessage(t, c, "INBOX.Warmbly", "<filed@test>")

	if err := c.MarkAsRead(context.Background(), "Warmbly", 1); err != nil {
		t.Fatalf("mark as read in the bare folder name: %v", err)
	}
	if err := c.MarkImportant(context.Background(), "Warmbly", 1); err != nil {
		t.Fatalf("mark important in the bare folder name: %v", err)
	}
	for _, sel := range wire.commands("SELECT") {
		if strings.Contains(sel, `"WARMBLY"`) || strings.HasSuffix(sel, " WARMBLY") {
			t.Fatalf("selected the bare folder name, which a prefixed server refuses: %s", sel)
		}
	}
	if len(wire.commands("SELECT")) == 0 {
		t.Fatal("no SELECT was sent")
	}
}

// INBOX is defined outside every namespace, so the prefix must never be put in
// front of it: "INBOX.INBOX" is a folder no server has, and a spam rescue
// names the inbox as its destination on exactly the servers that have a prefix.
func TestQualifyNeverPrefixesInbox(t *testing.T) {
	prefix := "INBOX."
	c := &Client{nsPrefix: &prefix}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range []string{"INBOX", "inbox", " Inbox "} {
		if got := c.qualifyMailboxLocked(name); got != name {
			t.Errorf("qualify(%q) = %q, want it untouched", name, got)
		}
	}
	if got := c.qualifyMailboxLocked("Warmbly"); got != "INBOX.Warmbly" {
		t.Errorf("qualify(Warmbly) = %q, want INBOX.Warmbly", got)
	}
	if got := c.qualifyMailboxLocked("INBOX.Warmbly"); got != "INBOX.Warmbly" {
		t.Errorf("an already-qualified name was prefixed again: %q", got)
	}
}

// A move names its source bare too, so the whole move has to resolve on a
// prefixed server end to end: SELECT the qualified source, MOVE to the
// qualified destination.
func TestMoveToFolderQualifiesBothEnds(t *testing.T) {
	c, wire := recordingServer(t, imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapMove: {}}, "INBOX.Junk")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	prefix := "INBOX."
	c.mu.Lock()
	c.nsPrefix = &prefix
	c.mu.Unlock()
	appendMessage(t, c, "INBOX.Junk", "<junk@test>")

	moved, err := c.MoveToFolder(context.Background(), "Junk", "Warmbly", 1)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !moved {
		t.Fatal("the message was not moved")
	}
	for _, line := range append(wire.commands("SELECT"), wire.commands("MOVE")...) {
		if strings.Contains(line, `"JUNK"`) || strings.Contains(line, `"WARMBLY"`) {
			t.Fatalf("a bare folder name reached the wire: %s", line)
		}
	}
	if n := len(wire.commands("MOVE")); n != 1 {
		t.Fatalf("expected one MOVE, got %d", n)
	}
}
