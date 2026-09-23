package config

import (
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"
)

// GoogleDirectoryScope reads a Workspace domain's user list, for picking which mailboxes to connect.
const GoogleDirectoryScope = "https://www.googleapis.com/auth/admin.directory.user.readonly"

// GoogleDelegationScopes are what a Workspace administrator authorizes for the
// service account: the Gmail scopes an OAuth mailbox holds, plus the directory.
var GoogleDelegationScopes = []string{gmail.GmailModifyScope, gmail.GmailSettingsBasicScope, GoogleDirectoryScope}

// GoogleDelegationKey is the service account key (JSON) Google Workspace
// domain-wide delegation mints tokens with, from GOOGLE_WORKSPACE_DELEGATION_KEY
// or the file GOOGLE_WORKSPACE_DELEGATION_KEY_FILE names. Empty turns the feature off.
func GoogleDelegationKey() []byte {
	if v := strings.TrimSpace(os.Getenv("GOOGLE_WORKSPACE_DELEGATION_KEY")); v != "" {
		return []byte(v)
	}
	if path := strings.TrimSpace(os.Getenv("GOOGLE_WORKSPACE_DELEGATION_KEY_FILE")); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return b
		}
	}
	return nil
}
