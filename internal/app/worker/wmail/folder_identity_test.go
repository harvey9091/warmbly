package wmail

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// mailboxUpdates is every folder MAILBOX_UPDATE relayed, by name.
func mailboxUpdates(events []captured) map[string]*models.Mailbox {
	out := map[string]*models.Mailbox{}
	for _, e := range events {
		if e.eventType != models.JobEventTypeMailboxUpdate {
			continue
		}
		box := e.body.(*models.JobEventMailboxUpdate).Data
		out[box.Name] = box
	}
	return out
}

func mailboxDeletes(events []captured) []string {
	var out []string
	for _, e := range events {
		if e.eventType == models.JobEventTypeMailboxDelete {
			out = append(out, e.body.(*models.JobEventMailboxDelete).Mailbox)
		}
	}
	return out
}

// The bug this file exists for: a server that derives UIDVALIDITY from a
// folder's creation time gives every folder made in the same second the same
// number, and a folder tree made by a mail client, an import or a migration
// is made in one second by definition. Keyed on that number, all but one of
// them were dropped and never synced. Keyed on the name, which is what IMAP
// actually guarantees, every one of them is followed.
func TestSyncFollowsEveryFolderSharingAUIDValidity(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100},
		{Name: "Clients", UIDValidity: 42, HighestModSeq: 100},
		{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100},
		{Name: "Clients/Globex", UIDValidity: 42, HighestModSeq: 100},
	}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	updates := mailboxUpdates(*events)
	for _, name := range []string{"Clients", "Clients/Acme", "Clients/Globex"} {
		if updates[name] == nil {
			t.Errorf("%q was never baselined; a shared UIDVALIDITY cost it its whole sync", name)
		}
	}
	if got := mailboxDeletes(*events); len(got) != 0 {
		t.Errorf("retired %v; nothing left the listing", got)
	}
	if len(w.SmtpImapData.Mailboxes) != 4 {
		t.Fatalf("tracked %d folders, want 4", len(w.SmtpImapData.Mailboxes))
	}
}

// A stored message carries both: the folder's name, which is its identity and
// survives a UIDVALIDITY change, and the UIDVALIDITY itself, which is the
// generation its UID belongs to.
func TestSyncStampsTheFolderNameOnStoredMail(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 200}},
		changed: uidRange(1),
	}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	var stored *models.EmailMessageStoreData
	for _, e := range *events {
		if e.eventType == models.JobEventTypeNewEmail {
			stored = e.body.(*models.JobEventNewEmail).Message
		}
	}
	if stored == nil {
		t.Fatal("no message was stored")
	}
	if stored.FolderPath != "Clients/Acme" {
		t.Errorf("FolderPath = %q, want the folder's name", stored.FolderPath)
	}
	if stored.Mailbox != 42 {
		t.Errorf("Mailbox = %d, want the UIDVALIDITY the uid belongs to", stored.Mailbox)
	}
}

// An IMAP RENAME keeps UIDVALIDITY and every UID, so a rename is a move, not
// a folder leaving and another arriving. Read the other way it would orphan
// the mail filed under the old name and re-import the folder's history.
func TestSyncFollowsAFolderRename(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100},
		{Name: "Clients/Acme Corp", UIDValidity: 42, HighestModSeq: 100},
	}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes,
		&models.Mailbox{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100})
	w.tracker.setFolder("Clients/Acme", models.SyncFolderCursor{UID: 900})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	var renamed *models.JobEventMailboxRename
	for _, e := range *events {
		if e.eventType == models.JobEventTypeMailboxRename {
			renamed = e.body.(*models.JobEventMailboxRename)
		}
	}
	if renamed == nil {
		t.Fatal("no MAILBOX_RENAME was relayed; the folder's mail would be orphaned")
	}
	if renamed.From != "Clients/Acme" || renamed.To != "Clients/Acme Corp" {
		t.Errorf("renamed %q -> %q, want Clients/Acme -> Clients/Acme Corp", renamed.From, renamed.To)
	}
	if got := mailboxDeletes(*events); len(got) != 0 {
		t.Errorf("retired %v; a rename must not delete the folder", got)
	}
	if cur := w.tracker.folder("Clients/Acme Corp"); cur.UID != 900 {
		t.Errorf("backfill floor under the new name = %d, want 900 to move with it", cur.UID)
	}
	if cur := w.tracker.folder("Clients/Acme"); cur.UID != 0 {
		t.Error("the backfill floor was left behind under the old name")
	}
	if len(w.SmtpImapData.Mailboxes) != 2 {
		t.Fatalf("tracked %d folders, want 2", len(w.SmtpImapData.Mailboxes))
	}
}

// On a server that stamps UIDVALIDITY from a creation time, several folders
// share a number, so "the folder carrying this UIDVALIDITY" can name more
// than one candidate. Guessing there would move a folder's mail into an
// unrelated folder, so an ambiguous match is not a rename at all.
func TestSyncDoesNotGuessARenameWhenTwoFoldersCouldBeIt(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "Clients/Acme Corp", UIDValidity: 42, HighestModSeq: 100},
		{Name: "Clients/Globex", UIDValidity: 42, HighestModSeq: 100},
	}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if hasEvent(*events, models.JobEventTypeMailboxRename) {
		t.Fatal("a rename was guessed from an ambiguous UIDVALIDITY")
	}
	if got := mailboxDeletes(*events); len(got) != 1 || got[0] != "Clients/Acme" {
		t.Errorf("retired %v, want the folder that left the listing", got)
	}
}

// A changed UIDVALIDITY is the server saying every UID we hold for the folder
// is void. The folder is re-baselined rather than followed from a cursor that
// now addresses nothing, and an unfinished import walks it again.
func TestSyncRebaselinesAFolderWhoseUIDValidityChanged(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{{Name: "INBOX", UIDValidity: 900, HighestModSeq: 5}},
		changed: uidRange(3),
	}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.tracker.setFolder("INBOX", models.SyncFolderCursor{UID: 500, Done: true})
	w.flagScan = map[string]*folderFlagScan{"INBOX": {}}

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	box := mailboxUpdates(*events)["INBOX"]
	if box == nil {
		t.Fatal("the re-baselined folder was never relayed")
	}
	if box.UIDValidity != 900 || box.HighestModSeq != 5 {
		t.Errorf("relayed %+v, want the server's current cursor", box)
	}
	if hasEvent(*events, models.JobEventTypeNewEmail) {
		t.Error("the folder was walked from a cursor its server had already voided")
	}
	if cur := w.tracker.folder("INBOX"); cur.UID != 0 || cur.Done {
		t.Errorf("backfill floor = %+v, want it cleared so the folder is walked again", cur)
	}
	if _, held := w.flagScan["INBOX"]; held {
		t.Error("the flag snapshot survived; it describes UIDs that no longer mean anything")
	}
	if got := mailboxDeletes(*events); len(got) != 0 {
		t.Errorf("retired %v; the folder is still there", got)
	}
}

// A folder's backfill floor goes with the folder. A name is reusable, so a
// floor left behind is inherited by whatever is created under that name next:
// a "done" cursor skips the new folder's history entirely, and the messages it
// skips are not new mail either, so nothing reports them missing.
func TestSyncForgetsTheBackfillFloorOfADeletedFolder(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100}}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes,
		&models.Mailbox{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100})
	w.tracker.setFolder("Clients/Acme", models.SyncFolderCursor{UID: 900, Done: true})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := mailboxDeletes(*events); len(got) != 1 || got[0] != "Clients/Acme" {
		t.Fatalf("retired %v, want the folder that left the listing", got)
	}
	if cur := w.tracker.folder("Clients/Acme"); cur.Done || cur.UID != 0 {
		t.Errorf("backfill floor = %+v, want it gone with the folder", cur)
	}
}

// A UIDVALIDITY that names two missing folders and one new one cannot say
// which was renamed. Picking either moves a folder's mail into a folder it has
// nothing to do with, so neither is claimed.
func TestSyncDoesNotGuessARenameWhenTwoStoredFoldersAreMissing(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "Clients/Initech", UIDValidity: 42, HighestModSeq: 100},
	}}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10},
		&models.Mailbox{Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes,
		&models.Mailbox{Name: "Clients/Globex", UIDValidity: 42, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if hasEvent(*events, models.JobEventTypeMailboxRename) {
		t.Fatal("a rename was guessed from a UIDVALIDITY that two missing folders share")
	}
	deletes := mailboxDeletes(*events)
	if len(deletes) != 2 {
		t.Fatalf("retired %v, want both folders that left the listing", deletes)
	}
	if mailboxUpdates(*events)["Clients/Initech"] == nil {
		t.Error("the new folder was not baselined")
	}
}
