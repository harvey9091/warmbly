package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// Issue #356: a folder is keyed by its name, not by its UIDVALIDITY, which
// RFC 3501 never promised was unique across folders. These run the statements
// that changed against a real schema, because the interesting parts of both
// are conditions Postgres evaluates and Go cannot: the upsert's new conflict
// target, and the rename's guard against a name the account already has.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveFolderIdentity -v

type folderFixture struct {
	org     uuid.UUID
	user    uuid.UUID
	mailbox uuid.UUID
}

func newFolderFixture(t *testing.T, pool *pgxpool.Pool) *folderFixture {
	t.Helper()
	ctx := context.Background()
	f := &folderFixture{org: uuid.New(), user: uuid.New(), mailbox: uuid.New()}
	tag := "i356-" + f.org.String()[:8]

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %q: %v", sql[:min(60, len(sql))], err)
		}
	}
	exec(`INSERT INTO users (id, first_name, last_name, email, password_hash)
	      VALUES ($1, 'Folder', 'Live', $2, 'x')`, f.user, tag+"@test.local")
	exec(`INSERT INTO organizations (id, name, slug, owner_user_id)
	      VALUES ($1, 'Issue 356', $2, $3)`, f.org, tag, f.user)
	exec(`INSERT INTO organization_members (organization_id, user_id, role, accepted_at)
	      VALUES ($1, $2, 'owner', NOW())`, f.org, f.user)
	exec(`INSERT INTO email_accounts (id, user_id, organization_id, email, name, signature_plain, signature_html, provider)
	      VALUES ($1, $2, $3, $4, 'Folder', '', '', 'smtp_imap')`, f.mailbox, f.user, f.org, tag+"-mb@test.local")

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM unibox_emails WHERE email_id = $1`, f.mailbox)
		_, _ = pool.Exec(c, `DELETE FROM unibox_mailboxes WHERE email_id = $1`, f.mailbox)
		_, _ = pool.Exec(c, `DELETE FROM email_accounts WHERE id = $1`, f.mailbox)
		_, _ = pool.Exec(c, `DELETE FROM organization_members WHERE organization_id = $1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM organizations WHERE id = $1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, f.user)
	})
	return f
}

func liveFolderDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB not set")
	}
	handle, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { handle.Pool.Close() })
	return handle
}

// Folders that share a UIDVALIDITY are ordinary folders, each with its own
// row and its own cursor. Under the old key the second upsert overwrote the
// first, which is how a folder lost its entire sync.
func TestLiveFolderIdentityKeepsFoldersSharingAUIDValidity(t *testing.T) {
	handle := liveFolderDB(t)
	f := newFolderFixture(t, handle.Pool)
	repo := NewMailboxRepository(handle)
	ctx := context.Background()

	for _, name := range []string{"Clients/Acme", "Clients/Globex"} {
		if err := repo.CreateEntry(ctx, f.user, f.mailbox, &models.Mailbox{
			Name: name, UIDValidity: 42, HighestModSeq: 7, Attrs: []string{},
		}); err != nil {
			t.Fatalf("CreateEntry(%q): %v", name, err)
		}
	}

	boxes, err := repo.ListMailboxes(ctx, f.user, f.mailbox)
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	if len(boxes) != 2 {
		t.Fatalf("stored %d folders, want both: %+v", len(boxes), boxes)
	}

	// The cursor is per folder, and re-upserting one leaves the other alone.
	if err := repo.CreateEntry(ctx, f.user, f.mailbox, &models.Mailbox{
		Name: "Clients/Acme", UIDValidity: 43, HighestModSeq: 99, Attrs: []string{},
	}); err != nil {
		t.Fatalf("CreateEntry (second pass): %v", err)
	}
	acme, err := repo.GetMailbox(ctx, f.user, f.mailbox, "Clients/Acme")
	if err != nil || acme == nil {
		t.Fatalf("GetMailbox(Clients/Acme) = %+v, %v", acme, err)
	}
	if acme.UIDValidity != 43 || acme.HighestModSeq != 99 {
		t.Errorf("Clients/Acme = %+v, want the new generation and cursor", acme)
	}
	globex, err := repo.GetMailbox(ctx, f.user, f.mailbox, "Clients/Globex")
	if err != nil || globex == nil {
		t.Fatalf("GetMailbox(Clients/Globex) = %+v, %v", globex, err)
	}
	if globex.HighestModSeq != 7 {
		t.Errorf("Clients/Globex cursor = %d, want 7 left alone", globex.HighestModSeq)
	}

	// Deleting one folder by name does not take its UIDVALIDITY twin with it.
	if err := repo.DeleteMailbox(ctx, f.user, f.mailbox, "Clients/Globex"); err != nil {
		t.Fatalf("DeleteMailbox: %v", err)
	}
	boxes, err = repo.ListMailboxes(ctx, f.user, f.mailbox)
	if err != nil {
		t.Fatalf("ListMailboxes after delete: %v", err)
	}
	if len(boxes) != 1 || boxes[0].Name != "Clients/Acme" {
		t.Fatalf("after deleting one folder: %+v", boxes)
	}
}

// A rename moves the row and the mail. It must not run when the account
// already has a folder under the new name, which is not a rename at all.
func TestLiveFolderIdentityRenameMovesRowAndMail(t *testing.T) {
	handle := liveFolderDB(t)
	f := newFolderFixture(t, handle.Pool)
	mailboxes := NewMailboxRepository(handle)
	unibox := NewUniboxRepository(handle)
	ctx := context.Background()

	if err := mailboxes.CreateEntry(ctx, f.user, f.mailbox, &models.Mailbox{
		Name: "Clients/Acme", UIDValidity: 42, HighestModSeq: 7, Attrs: []string{},
	}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	msg := &models.EmailMessageStoreData{
		ID: uuid.New(), EmailID: f.mailbox, Mailbox: 42, FolderPath: "Clients/Acme",
		ThreadID: "t-356", MessageID: "<356@test>", UID: 5, Folder: models.FolderInbox,
	}
	if err := unibox.CreateEntry(ctx, f.user, msg); err != nil {
		t.Fatalf("CreateEntry(message): %v", err)
	}

	renamed, err := mailboxes.RenameMailbox(ctx, f.user, f.mailbox, "Clients/Acme", "Clients/Acme Corp")
	if err != nil {
		t.Fatalf("RenameMailbox: %v", err)
	}
	if !renamed {
		t.Fatal("RenameMailbox reported nothing moved")
	}

	moved, err := mailboxes.GetMailbox(ctx, f.user, f.mailbox, "Clients/Acme Corp")
	if err != nil || moved == nil {
		t.Fatalf("GetMailbox after rename = %+v, %v", moved, err)
	}
	// The cursor rides along: an IMAP RENAME changes nothing about the UIDs,
	// so re-baselining the folder would re-import its history for a label.
	if moved.UIDValidity != 42 || moved.HighestModSeq != 7 {
		t.Errorf("renamed folder = %+v, want its cursor intact", moved)
	}
	stored, err := unibox.GetByID(ctx, f.user, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.FolderPath != "Clients/Acme Corp" {
		t.Errorf("message folder_path = %q, want it to move with the folder", stored.FolderPath)
	}

	// A name the account already has is not a rename; the row stays put
	// rather than colliding with the folder that is already there.
	if err := mailboxes.CreateEntry(ctx, f.user, f.mailbox, &models.Mailbox{
		Name: "Clients/Globex", UIDValidity: 43, Attrs: []string{},
	}); err != nil {
		t.Fatalf("CreateEntry(Globex): %v", err)
	}
	onto, err := mailboxes.RenameMailbox(ctx, f.user, f.mailbox, "Clients/Globex", "Clients/Acme Corp")
	if err != nil {
		t.Fatalf("RenameMailbox onto an existing name: %v", err)
	}
	if onto {
		t.Fatal("a rename onto an occupied name reported success")
	}
	still, err := mailboxes.GetMailbox(ctx, f.user, f.mailbox, "Clients/Globex")
	if err != nil || still == nil {
		t.Fatalf("Clients/Globex was moved onto an occupied name: %+v, %v", still, err)
	}
	// The mail must not have moved either: the two halves of a rename travel
	// together or the messages end up in a folder nothing renamed.
	stayed, err := unibox.GetByID(ctx, f.user, msg.ID)
	if err != nil {
		t.Fatalf("GetByID after the refused rename: %v", err)
	}
	if stayed.FolderPath != "Clients/Acme Corp" {
		t.Errorf("message folder_path = %q; the refused rename moved mail", stayed.FolderPath)
	}
}
