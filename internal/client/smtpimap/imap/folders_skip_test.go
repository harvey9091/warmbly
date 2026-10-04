package imap

import (
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
)

func TestSkipsFolder(t *testing.T) {
	skip := []string{"Warmer", "Clients/Acme", " inbox "}
	cases := []struct {
		name string
		box  models.Mailbox
		want bool
	}{
		{"exact", models.Mailbox{Name: "Warmer", Delim: "/"}, true},
		{"case does not count", models.Mailbox{Name: "WARMER", Delim: "/"}, true},
		{"subfolder", models.Mailbox{Name: "Warmer/Replies", Delim: "/"}, true},
		{"subfolder under a dot server", models.Mailbox{Name: "Warmer.Replies", Delim: "."}, true},
		{"nested entry", models.Mailbox{Name: "Clients/Acme/2026", Delim: "/"}, true},
		{"prefix without a delimiter is a different folder", models.Mailbox{Name: "Warmer2", Delim: "/"}, false},
		{"no delimiter reported means exact only", models.Mailbox{Name: "Warmer/Replies", Delim: ""}, false},
		{"another folder", models.Mailbox{Name: "Receipts", Delim: "/"}, false},
		{"INBOX is never skipped, even when listed", models.Mailbox{Name: "INBOX", Delim: "/"}, false},
		{"an INBOX entry does not take the inbox's subfolders", models.Mailbox{Name: "INBOX/Work", Delim: "/"}, false},
		{"special-use attribute wins over the name", models.Mailbox{Name: "Warmer", Delim: "/", Attrs: []string{"\\Sent"}}, false},
		{"special name wins", models.Mailbox{Name: "Sent", Delim: "/"}, false},
	}
	for _, tc := range cases {
		if got := SkipsFolder(tc.box, skip); got != tc.want {
			t.Errorf("%s: SkipsFolder(%q) = %v, want %v", tc.name, tc.box.Name, got, tc.want)
		}
	}
	if SkipsFolder(models.Mailbox{Name: "Warmer"}, nil) {
		t.Error("an empty list skips nothing")
	}
}

func TestNormalizeSkipFolders(t *testing.T) {
	got, xerr := NormalizeSkipFolders([]string{" Warmer ", "", "warmer", "Clients/Acme", "INBOX/Newsletters"})
	if xerr != nil {
		t.Fatalf("unexpected refusal: %v", xerr)
	}
	want := []string{"Warmer", "Clients/Acme", "INBOX/Newsletters"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("normalized %v, want %v", got, want)
	}

	refused := [][]string{
		{"INBOX"},
		{"inbox"},
		{"Sent"},
		{"INBOX.Drafts"},
		{"Junk"},
		{"[Gmail]/Trash"},
		{"Archive"},
		{"bad\x01name"},
		{strings.Repeat("x", config.SyncSkipFolderNameMax+1)},
	}
	for _, in := range refused {
		if _, xerr := NormalizeSkipFolders(in); xerr == nil {
			t.Errorf("NormalizeSkipFolders(%q) accepted, want a refusal", in)
		} else if xerr.Identifier != "invalid_sync_folder" {
			t.Errorf("NormalizeSkipFolders(%q) refused as %q, want invalid_sync_folder", in, xerr.Identifier)
		}
	}

	many := make([]string, config.SyncSkipFoldersMax+1)
	for i := range many {
		many[i] = "Folder" + strings.Repeat("x", i)
	}
	if _, xerr := NormalizeSkipFolders(many); xerr == nil {
		t.Error("a list over the cap was accepted")
	}
	if got, _ := NormalizeSkipFolders(nil); got == nil || len(got) != 0 {
		t.Errorf("nil in, want an empty list out, got %#v", got)
	}
}
