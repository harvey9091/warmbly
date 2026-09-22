package wmail

import (
	"context"
	"fmt"
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
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10})
	// The previous pass of this session listed three messages.
	w.rememberListing(&models.Mailbox{Name: "INBOX", Messages: 3, UIDNext: 10})
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

	// Next pass another message leaves, for somewhere that is not skipped.
	// Only that row is searched for: the one deleted for good last pass is
	// remembered, and the one already removed is not looked for twice.
	conn.folders[0].Messages = 1
	conn.all = []goimap.UID{10}
	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if conn.finds != 3 {
		t.Fatalf("searched %d times over two passes, want 3: a settled row was searched for again", conn.finds)
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
	w, events := newIMAPTestMail(conn, &fixedBudget{allow: 10}, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10})
	w.rememberListing(&models.Mailbox{Name: "INBOX", Messages: 3, UIDNext: 10})
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
		name   string
		before imapListed
		now    models.Mailbox
		want   bool
	}{
		{"nothing changed", imapListed{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 3, UIDNext: 10}, false},
		{"one arrived", imapListed{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 4, UIDNext: 11}, false},
		{"one left", imapListed{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 2, UIDNext: 10}, true},
		{"one arrived and one left", imapListed{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 3, UIDNext: 11}, true},
		{"empty before, two arrived, one left", imapListed{Messages: 0, UIDNext: 10}, models.Mailbox{Messages: 1, UIDNext: 12}, true},
		{"cursor went backwards", imapListed{Messages: 3, UIDNext: 10}, models.Mailbox{Messages: 1, UIDNext: 5}, false},
	}
	for _, tc := range cases {
		if got := imapMovedOut(tc.before, &tc.now); got != tc.want {
			t.Errorf("%s: imapMovedOut = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The first listing of a session only baselines: a folder whose stored
// cursor came from the control plane carries no count, so nothing is
// reconciled until a second listing can be compared with the first.
func TestImapSyncBaselinesCountsOnFirstListing(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{
			{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10, Messages: 1, Delim: "/"},
			{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		},
		all:       []goimap.UID{8},
		inSkipped: map[string]map[string]uint32{"Warmer": {"<7@fake.test>": 3}},
	}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10})
	ctx := &fakeSyncContext{stored: map[string][]repository.StoredFolderMessage{
		"INBOX": {{UID: 7, MessageID: "<7@fake.test>", ID: uuid.New()}},
	}}
	w.SyncContext = ctx

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if ctx.calls != 0 || len(removeIDs(*events)) != 0 {
		t.Fatalf("reconciled on the first listing: lookups=%d removed=%d", ctx.calls, len(removeIDs(*events)))
	}
	// The second listing shows the departure against the first.
	conn.folders[0].Messages = 0
	conn.all = nil
	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(removeIDs(*events)) != 1 {
		kinds := []models.JobEventType{}
		for _, e := range *events {
			kinds = append(kinds, e.eventType)
		}
		t.Fatalf("removed %d rows on the second listing, want 1 (lookups=%d finds=%d events=%v listed=%+v)", len(removeIDs(*events)), ctx.calls, conn.finds, kinds, w.listed)
	}
}

// A synced folder renamed into the skipped subtree keeps its UIDVALIDITY,
// but the rename matcher never sees the new name because skipped folders
// leave the listing first. The retirement still carries the marker, so the
// mail stored under the old name is purged like any skipped folder's.
func TestImapSyncMarksFolderRenamedIntoSkippedSubtree(t *testing.T) {
	conn := &fakeImapConn{folders: []models.Mailbox{
		{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		{Name: "Warmer/Leads", UIDValidity: 21, HighestModSeq: 50, Delim: "/"},
		{Name: "Receipts", UIDValidity: 33, HighestModSeq: 10, Delim: "/"},
	}}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100})
	w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes,
		&models.Mailbox{Name: "Leads", UIDValidity: 21, HighestModSeq: 50},
		// Gone for good, and its UIDVALIDITY matches nothing skipped.
		&models.Mailbox{Name: "Old", UIDValidity: 44, HighestModSeq: 5},
	)

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	got := map[string]bool{}
	for _, e := range mailboxEvents(*events, models.JobEventTypeMailboxDelete) {
		del := e.body.(*models.JobEventMailboxDelete)
		got[del.Mailbox] = del.Skipped
	}
	if skipped, ok := got["Leads"]; !ok || !skipped {
		t.Fatalf("Leads retired as %v (present %v), want skipped", skipped, ok)
	}
	if skipped, ok := got["Old"]; !ok || skipped {
		t.Fatalf("Old retired as %v (present %v), want a plain deletion", skipped, ok)
	}
	if len(mailboxEvents(*events, models.JobEventTypeMailboxRename)) != 0 {
		t.Fatal("a rename into a skipped folder must not be followed")
	}
}

// A row whose Message-ID was fetched this pass is live under a new UID,
// whatever a copy in a skipped folder says; it is never removed.
func TestImapSyncKeepsRowRefetchedThisPass(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{
			{Name: "INBOX", UIDValidity: 7, HighestModSeq: 200, UIDNext: 12, Messages: 1, Delim: "/"},
			{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		},
		changed:   []goimap.UID{11},
		all:       []goimap.UID{11},
		inSkipped: map[string]map[string]uint32{"Warmer": {"<11@fake.test>": 3}},
	}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 10})
	w.rememberListing(&models.Mailbox{Name: "INBOX", Messages: 1, UIDNext: 10})
	w.EmailMessageMapRepository = knownMessageMap{id: uuid.New().String()}
	// The stored row is the same message under its old UID: re-appended
	// this pass as UID 11 (a warmup engagement leg moved it out and back).
	w.SyncContext = &fakeSyncContext{stored: map[string][]repository.StoredFolderMessage{
		"INBOX": {{UID: 5, MessageID: "<11@fake.test>", ID: uuid.New()}},
	}}

	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if n := len(removeIDs(*events)); n != 0 {
		t.Fatalf("removed %d rows, want none: the message is live under a new UID", n)
	}
	if conn.finds != 0 {
		t.Fatalf("searched %d times for a row fetched this pass", conn.finds)
	}
}

// The reconciliation spends at most imapSkipSearchesPerPass searches on one
// folder per pass and continues on the next, so a folder emptied by hand is
// examined in slices rather than in one long tick.
func TestImapSyncCapsSkippedSearchesPerPass(t *testing.T) {
	conn := &fakeImapConn{
		folders: []models.Mailbox{
			{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 500, Messages: 0, Delim: "/"},
			{Name: "Warmer", UIDValidity: 9, HighestModSeq: 100, Delim: "/"},
		},
		inSkipped: map[string]map[string]uint32{"Warmer": {}},
	}
	budget := &skipBudget{fixedBudget: &fixedBudget{allow: 10}, skip: []string{"Warmer"}}
	w, events := newIMAPTestMail(conn, budget, &models.Mailbox{Name: "INBOX", UIDValidity: 7, HighestModSeq: 100, UIDNext: 500})
	w.rememberListing(&models.Mailbox{Name: "INBOX", Messages: 120, UIDNext: 500})
	w.EmailMessageMapRepository = knownMessageMap{id: uuid.New().String()}
	stored := make([]repository.StoredFolderMessage, 0, 120)
	for i := 1; i <= 120; i++ {
		id := fmt.Sprintf("<%d@fake.test>", i)
		stored = append(stored, repository.StoredFolderMessage{UID: uint32(i), MessageID: id, ID: uuid.New()})
	}
	// The last one is in the skipped folder; it is reached on the third pass.
	conn.inSkipped["Warmer"]["<120@fake.test>"] = 9
	ctx := &fakeSyncContext{stored: map[string][]repository.StoredFolderMessage{"INBOX": stored}}
	w.SyncContext = ctx

	for pass, wantFinds := range []int{imapSkipSearchesPerPass, 2 * imapSkipSearchesPerPass, 120} {
		if err := w.Sync(t.Context()); err != nil {
			t.Fatalf("pass %d: %v", pass+1, err)
		}
		if conn.finds != wantFinds {
			t.Fatalf("pass %d: %d searches so far, want %d", pass+1, conn.finds, wantFinds)
		}
	}
	if n := len(removeIDs(*events)); n != 1 {
		t.Fatalf("removed %d rows over three passes, want the one found in Warmer", n)
	}
	if w.skipPending["INBOX"] {
		t.Fatal("the folder is still marked pending after every row was examined")
	}
	// A fourth pass with nothing new does not look again.
	if err := w.Sync(t.Context()); err != nil {
		t.Fatalf("fourth pass: %v", err)
	}
	if conn.finds != 120 {
		t.Fatalf("a settled folder was searched again (finds = %d)", conn.finds)
	}
}
