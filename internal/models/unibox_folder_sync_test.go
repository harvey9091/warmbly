package models

import "testing"

// The rule that makes Archive/Delete in the thread header survive a sync.
// Every case below is a real event shape the consumer sees; the third is the
// one the feature exists for, and the fourth is the one a naive guard breaks.
func TestResolveFolderSync(t *testing.T) {
	cases := []struct {
		name           string
		storedFolder   string
		storedProvider string
		reported       string
		wantFolder     string
		wantProvider   string
		wantChanged    bool
	}{
		{
			name:         "a worker predating the folder field changes nothing",
			storedFolder: FolderInbox, storedProvider: FolderInbox, reported: "",
			wantFolder: FolderInbox, wantProvider: FolderInbox, wantChanged: false,
		},
		{
			name:         "an unknown folder changes nothing",
			storedFolder: FolderInbox, storedProvider: FolderInbox, reported: "starred",
			wantFolder: FolderInbox, wantProvider: FolderInbox, wantChanged: false,
		},
		{
			name:         "a flag scan does not undo a message filed in Warmbly",
			storedFolder: FolderTrash, storedProvider: FolderInbox, reported: FolderInbox,
			wantFolder: FolderTrash, wantProvider: FolderInbox, wantChanged: false,
		},
		{
			name:         "the provider moving it out of the inbox still wins",
			storedFolder: FolderArchive, storedProvider: FolderInbox, reported: FolderSpam,
			wantFolder: FolderSpam, wantProvider: FolderSpam, wantChanged: true,
		},
		{
			name:         "un-archiving at the provider reaches a locally archived message",
			storedFolder: FolderArchive, storedProvider: FolderArchive, reported: FolderInbox,
			wantFolder: FolderInbox, wantProvider: FolderInbox, wantChanged: true,
		},
		{
			name:         "an ordinary provider move is followed",
			storedFolder: FolderInbox, storedProvider: FolderInbox, reported: FolderTrash,
			wantFolder: FolderTrash, wantProvider: FolderTrash, wantChanged: true,
		},
		{
			name:         "a row written before provider_folder existed adopts",
			storedFolder: FolderInbox, storedProvider: "", reported: FolderInbox,
			wantFolder: FolderInbox, wantProvider: FolderInbox, wantChanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			folder, provider, changed := ResolveFolderSync(tc.storedFolder, tc.storedProvider, tc.reported)
			if folder != tc.wantFolder || provider != tc.wantProvider || changed != tc.wantChanged {
				t.Fatalf("ResolveFolderSync(%q, %q, %q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.storedFolder, tc.storedProvider, tc.reported,
					folder, provider, changed, tc.wantFolder, tc.wantProvider, tc.wantChanged)
			}
		})
	}
}

func TestFilableFolderRefusesProviderVerdicts(t *testing.T) {
	for _, f := range []string{FolderInbox, FolderArchive, FolderTrash} {
		if !FilableFolder(f) {
			t.Errorf("FilableFolder(%q) = false, want true", f)
		}
	}
	for _, f := range []string{FolderSent, FolderDrafts, FolderSpam, "", "Inbox"} {
		if FilableFolder(f) {
			t.Errorf("FilableFolder(%q) = true, want false", f)
		}
	}
}
