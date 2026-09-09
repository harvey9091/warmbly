package wmail

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// threadParentID decides which conversation an inbound message joins. A wrong
// answer does not lose the message, it files it under the wrong conversation,
// which is harder to notice and worse to work in.
//
// The case this pins: Reply-To as a fallback parent. It is an address header,
// not a message identifier, so keying on it puts every message a sender ever
// sent into one thread.
func TestThreadParentIDIgnoresReplyTo(t *testing.T) {
	cases := []struct {
		name string
		msg  *models.EmailMessageData
		want string
	}{
		{
			name: "a genuine reply threads on In-Reply-To",
			msg:  &models.EmailMessageData{InReplyTo: []string{"<parent@example.com>"}},
			want: "<parent@example.com>",
		},
		{
			name: "the last In-Reply-To wins on a deep chain",
			msg: &models.EmailMessageData{InReplyTo: []string{
				"<root@example.com>", "<latest@example.com>",
			}},
			want: "<latest@example.com>",
		},
		{
			name: "Reply-To alone is not a parent",
			msg:  &models.EmailMessageData{ReplyTo: []string{"reports@example.net"}},
			want: "",
		},
		{
			name: "Reply-To never overrides In-Reply-To",
			msg: &models.EmailMessageData{
				InReplyTo: []string{"<parent@example.com>"},
				ReplyTo:   []string{"someone@example.net"},
			},
			want: "<parent@example.com>",
		},
		{
			name: "a message that answers nothing has no parent",
			msg:  &models.EmailMessageData{},
			want: "",
		},
		{
			name: "nil is not a panic",
			msg:  nil,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := threadParentID(tc.msg); got != tc.want {
				t.Errorf("threadParentID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Two unrelated messages from one sender must not share a parent, which is the
// shape of the bug: same Reply-To, different conversations.
func TestThreadParentIDKeepsUnrelatedMessagesApart(t *testing.T) {
	sender := []string{"Aggregate Reports <reports@example.net>"}
	first := &models.EmailMessageData{MessageID: "<a@example.net>", ReplyTo: sender}
	second := &models.EmailMessageData{MessageID: "<b@example.net>", ReplyTo: sender}

	if p := threadParentID(first); p != "" {
		t.Fatalf("first message got parent %q, want none", p)
	}
	if p := threadParentID(second); p != "" {
		t.Fatalf("second message got parent %q, want none", p)
	}
}
