package wmail

import (
	"context"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// The sent copy the worker files for a campaign send is appended \Seen: the
// sender wrote it. Every provider maps read state onto the same flag, so the
// stored message has to take it from there rather than arriving unread and
// putting the customer's own outbound mail in their Unread view.
func TestStoredReadStateFollowsTheProvider(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
		want  bool
	}{
		{"a sent copy filed \\Seen", []string{models.FlagSeen}, true},
		{"new mail nobody has opened", []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, events := newIMAPTestMail(&fakeImapConn{}, &fixedBudget{}, &models.Mailbox{})
			msg := &models.EmailMessageData{MessageID: "<sent@test>", Flags: tc.flags}

			for _, store := range map[string]func(context.Context, *models.EmailMessageData) error{
				"imap": w.imapStore, "google": w.googleStore, "graph": w.graphStore,
			} {
				if err := store(context.Background(), msg); err != nil {
					t.Fatal(err)
				}
			}

			for _, e := range *events {
				if e.eventType != models.JobEventTypeNewEmail {
					continue
				}
				if got := e.body.(*models.JobEventNewEmail).Message.Seen; got != tc.want {
					t.Errorf("stored seen = %v, want %v", got, tc.want)
				}
			}
			if len(*events) == 0 {
				t.Fatal("no NEW_EMAIL was relayed")
			}
		})
	}
}
