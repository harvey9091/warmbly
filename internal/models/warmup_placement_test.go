package models

import (
	"testing"

	"github.com/warmbly/warmbly/internal/config"
)

func TestWarmupFiling(t *testing.T) {
	cases := []struct {
		name          string
		placement     string
		folder        string
		wantPlacement string
		wantFolder    string
	}{
		{"unset row adopts the default", "", "", WarmupPlacementFolder, config.WarmupFolderDefault},
		{"garbage placement adopts the default", "sideways", "", WarmupPlacementFolder, config.WarmupFolderDefault},
		{"named folder is kept", WarmupPlacementFolder, "Reputation", WarmupPlacementFolder, "Reputation"},
		{"blank folder falls back even when the mode is set", WarmupPlacementFolder, "   ", WarmupPlacementFolder, config.WarmupFolderDefault},
		{"inbox keeps a folder name for a later switch back", WarmupPlacementInbox, "Reputation", WarmupPlacementInbox, "Reputation"},
		{"archive is carried through", WarmupPlacementArchive, "", WarmupPlacementArchive, config.WarmupFolderDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Email{WarmupPlacement: tc.placement, WarmupFolder: tc.folder}
			placement, folder := e.WarmupFiling()
			if placement != tc.wantPlacement || folder != tc.wantFolder {
				t.Fatalf("WarmupFiling() = (%q, %q), want (%q, %q)", placement, folder, tc.wantPlacement, tc.wantFolder)
			}
		})
	}
}

func TestValidWarmupPlacement(t *testing.T) {
	for _, ok := range []string{WarmupPlacementFolder, WarmupPlacementInbox, WarmupPlacementArchive} {
		if !ValidWarmupPlacement(ok) {
			t.Errorf("%q should be a valid placement", ok)
		}
	}
	for _, bad := range []string{"", "Folder", "trash", "spam"} {
		if ValidWarmupPlacement(bad) {
			t.Errorf("%q should not be a valid placement", bad)
		}
	}
}
