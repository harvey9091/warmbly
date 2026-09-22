package wmail

import (
	"context"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

func (c *fakeImapConn) FindUIDByMessageID(_ context.Context, folder, messageID string) (uint32, error) {
	c.finds++
	return c.inSkipped[folder][messageID], nil
}

// skipBudget is fixedBudget with an owner's skip list in the policy.
type skipBudget struct {
	*fixedBudget
	skip []string
}

func (b *skipBudget) Policy() models.SyncPolicy {
	return normalizePolicy(models.SyncPolicy{SkipFolders: b.skip})
}

func mailboxEvents(events []captured, kind models.JobEventType) []captured {
	var out []captured
	for _, e := range events {
		if e.eventType == kind {
			out = append(out, e)
		}
	}
	return out
}

// A folder on the skip list is never baselined and never fetched, and its
// subfolders go with it; a folder that merely shares the prefix does not.
func TestImapSyncNeverBaselinesSkippedFolders(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer/Replies", UIDValidity: 10, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer2", UIDValidity: 11, HighestModSeq: 100, Delim: "/"},
	}}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	baselined := map[string]bool{}
	for _, e := range mailboxEvents(*events, models.JobEventTypeMailboxUpdate) {
		baselined[e.body.(*models.JobEventMailboxUpdate).Data.Name] = true
	}
	if baselined["Warmer"] || baselined["Warmer/Replies"] {
		t.Fatalf("a skipped folder was baselined: %v", baselined)
	}
	if !baselined["Warmer2"] {
		t.Fatal("Warmer2 shares a prefix but is a different folder; it must be synced")
	}
	if len(mailboxEvents(*events, models.JobEventTypeMailboxDelete)) != 0 {
		t.Fatal("nothing was tracked for the skipped folders, so nothing should be retired")
	}
	if len(w.SmtpImapData.Mailboxes) != 2 {
		t.Fatalf("tracked %d folders, want INBOX and Warmer2", len(w.SmtpImapData.Mailboxes))
	}
}

// A folder synced before the owner excluded it is retired like one the
// server dropped, with the marker that takes its stored mail along.
func TestImapSyncRetiresNewlySkippedFolder(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
	}}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes, &models.Mailbox{Name: "Warmer", UIDValidity: 9, HighestModSeq: 90})
	w.tracker.setFolder("Warmer", models.SyncFolderCursor{UID: 40})

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	deletes := mailboxEvents(*events, models.JobEventTypeMailboxDelete)
	if len(deletes) != 1 {
		t.Fatalf("got %d MAILBOX_DELETE events, want 1", len(deletes))
	}
	del := deletes[0].body.(*models.JobEventMailboxDelete)
	if del.Mailbox != "Warmer" || !del.Skipped {
		t.Fatalf("retired %+v, want Warmer with the skipped marker", del)
	}
	if len(w.SmtpImapData.Mailboxes) != 1 || w.SmtpImapData.Mailboxes[0].Name != "INBOX" {
		t.Fatalf("tracked %v, want just INBOX", w.SmtpImapData.Mailboxes)
	}
	if cur := w.tracker.folder("Warmer"); cur.UID != 0 {
		t.Fatalf("the backfill floor for Warmer survived: %+v", cur)
	}
	if conn.fetches != 0 {
		t.Fatalf("fetched %d batches from a skipped folder, want none", conn.fetches)
	}
}

// Mail that left a synced folder is removed only when it turns up in a
// skipped folder; mail that left for anywhere else is kept, and is not
// searched for again on the next pass.
func TestImapSyncRemovesMailMovedIntoSkippedFolder(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{
			// Three messages were here last pass; one arrived and two left.
			{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 11, Messages: 2, Delim: "/"},
			{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		},
		all:       []goimap.UID{8, 10},
		inSkipped: map[string]map[string]uint32{"Warmer": {"<9@fake.test>": 3}},
	}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10, Messages: 3})
	w.EmailMessageMapRepository = knownMessageMap{id: uuid.New().String()}
	warmup, deleted, kept := uuid.New(), uuid.New(), uuid.New()
	w.SyncContext = &fakeSyncContext{stored: map[string][]repository.StoredFolderMessage{
		"INBOX": {
			{UID: 7, MessageID: "<7@fake.test>", ID: deleted},
			{UID: 8, MessageID: "<8@fake.test>", ID: kept},
			{UID: 9, MessageID: "<9@fake.test>", ID: warmup},
		},
	}}

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	removed := removeIDs(*events)
	if len(removed) != 1 || removed[0] != warmup {
		t.Fatalf("removed %v, want exactly the message found in Warmer (%s)", removed, warmup)
	}
	for _, e := range mailboxEvents(*events, models.JobEventTypeRemoveEmail) {
		if got := e.body.(*models.JobEventRemoveEmail).SkippedFolder; got != "Warmer" {
			t.Fatalf("removal named folder %q, want Warmer", got)
		}
	}
	if conn.finds != 2 {
		t.Fatalf("searched the skipped folder %d times, want once per vanished row (2)", conn.finds)
	}

	// Next pass, nothing else changed: the row that was deleted for good is
	// remembered and not searched for again.
	conn.folders[0].Messages = 2
	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if conn.finds != 2 {
		t.Fatalf("a settled row was searched for again (finds = %d)", conn.finds)
	}
	if len(removeIDs(*events)) != 1 {
		t.Fatal("the second pass removed something")
	}
}

// Without a skip list the count check is off entirely: a folder the owner
// emptied costs no lookups and loses no rows.
func TestImapSyncLeavesVanishedMailAloneWithoutSkipList(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10, Messages: 1, Delim: "/"}},
		all:     []goimap.UID{8},
	}
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10}, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10, Messages: 3})
	ctx := &fakeSyncContext{stored: map[string][]repository.StoredFolderMessage{
		"INBOX": {{UID: 7, MessageID: "<7@fake.test>", ID: uuid.New()}},
	}}
	w.SyncContext = ctx

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if ctx.calls != 0 || conn.finds != 0 || len(removeIDs(*events)) != 0 {
		t.Fatalf("folder rows were reconciled without a skip list: lookups=%d finds=%d removed=%d", ctx.calls, conn.finds, len(removeIDs(*events)))
	}
}

func TestImapMovedOut(t *testing.T) {
	cases := []struct {
		name        string
		before, now models.Mailbox
		want        bool
	}{
		{"nothing changed", models.Mailbox{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 3, UIDNext: 10}, false},
		{"one arrived", models.Mailbox{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 4, UIDNext: 11}, false},
		{"one left", models.Mailbox{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 2, UIDNext: 10}, true},
		{"one arrived and one left", models.Mailbox{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 3, UIDNext: 11}, true},
		{"no baseline from this session", models.Mailbox{Messages: 0, UIDNext: 10}, models.Mailbox{Messages: 2, UIDNext: 10}, false},
		{"cursor went backwards", models.Mailbox{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 1, UIDNext: 5}, false},
	}
	for _, tc := range cases {
		if got := imapMovedOut(&tc.before, &tc.now); got != tc.want {
			t.Errorf("%s: imapMovedOut = %v, want %v", tc.name, got, tc.want)
		}
	}
}
