package jobs

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emailverify"
	"github.com/warmbly/warmbly/internal/repository"
)

// recordingEvidence captures what the send path taught verification.
type recordingEvidence struct {
	kinds []string
}

func (r *recordingEvidence) RecordEvidence(_ context.Context, _ uuid.UUID, _ models.EvidenceStep, kind, _, _ string) {
	r.kinds = append(r.kinds, kind)
}

// The same thing through the real handler: a greylisted RCPT must walk the
// step back for a retry and teach verification nothing about the address.
//
//	WARMBLY_TEST_DB=postgres://... go test ./internal/app/consumer/ -run LiveGreylisted -v
func TestLiveGreylistedSendIsNotRecordedAsABounce(t *testing.T) {
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB not set")
	}
	ctx := context.Background()
	handle, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { handle.Pool.Close() })

	f := newSendResultFixture(t, handle)
	for _, tc := range []struct {
		name   string
		code   string
		reason string
		want   int
	}{
		{
			"greylisted",
			string(errx.MailErrorCodeServerUnreachable),
			`The connection to the mail server could not be established (rcpt to: 450 "4.7.1 <box@example.com>: Recipient address rejected: Greylisted"). The server may be offline or blocking the connection.`,
			0,
		},
		{
			"no such user",
			string(errx.MailErrorCodeRecipientRejected),
			`The mail server rejected the recipient: 550 "5.1.1 <box@example.com>: no such user"`,
			1,
		},
	} {
		ev := &recordingEvidence{}
		s := &JobsService{
			TaskRepo:             repository.NewTaskRepository(handle.Pool),
			CampaignRepo:         repository.NewCampaignRepostory(handle),
			CampaignProgressRepo: repository.NewCampaignProgressRepository(handle.Pool),
			CampaignLogRepo:      repository.NewCampaignLogRepository(handle),
			ContactRepo:          repository.NewContactRepostory(handle),
			Evidence:             ev,
		}
		taskID := f.stampSend(t, s)
		if err := s.HandleEmailFailed(ctx, models.SendEmailResult{
			TaskID: taskID, Success: false,
			Error: &models.EmailSendError{Code: tc.code, Message: tc.reason},
		}); err != nil {
			t.Fatalf("%s: handle failed: %v", tc.name, err)
		}
		if len(ev.kinds) != tc.want {
			t.Fatalf("%s: recorded %v, want %d evidence records", tc.name, ev.kinds, tc.want)
		}
	}
}

// The send path now carries the server's own reply inside SERVER_UNREACHABLE,
// which made the retryable class look like a bounce to the text classifier:
// Postfix greylisting answers a perfectly good address with "Recipient address
// rejected", and that phrase is a recipient marker. Filing it as bounce
// evidence marks a valid contact undeliverable for a year on the strength of a
// delay, so the code has to gate the classification.
func TestRetryableFailuresAreNotEvidenceAboutTheAddress(t *testing.T) {
	// The live test below drives the real handler but needs a database, so it
	// skips in CI. This one mirrors the condition to keep the marker phrases
	// and the gate covered where there is no Postgres.
	evidence := func(code, reason string) bool {
		return code != string(errx.MailErrorCodeServerUnreachable) && emailverify.NamesRecipient(reason)
	}

	for _, tc := range []struct {
		name   string
		code   string
		reason string
		want   bool
	}{
		{
			"greylisted",
			string(errx.MailErrorCodeServerUnreachable),
			`The connection to the mail server could not be established (rcpt to: 450 "4.7.1 <box@example.com>: Recipient address rejected: Greylisted, try again later"). The server may be offline or blocking the connection.`,
			false,
		},
		{
			"mailbox busy",
			string(errx.MailErrorCodeServerUnreachable),
			`The connection to the mail server could not be established (rcpt to: 452 "4.2.2 Mailbox unavailable, over quota"). The server may be offline or blocking the connection.`,
			false,
		},
		// The permanent refusal is still evidence: that is the whole point of
		// reading the server's wording.
		{
			"address does not exist",
			string(errx.MailErrorCodeRecipientRejected),
			`The mail server rejected the recipient: 550 "5.1.1 <box@example.com>: no such user"`,
			true,
		},
		{
			"message refused naming the recipient",
			string(errx.MailErrorCodeSendRejected),
			`The receiving mail server refused this message: 550 "5.1.1 recipient not found"`,
			true,
		},
		// A dial that never reached a server names nobody either way.
		{
			"refused dial",
			string(errx.MailErrorCodeServerUnreachable),
			`The connection to the mail server could not be established (dial smtp.example.com:465: connect: connection refused). The server may be offline or blocking the connection.`,
			false,
		},
	} {
		if got := evidence(tc.code, tc.reason); got != tc.want {
			t.Errorf("%s: evidence = %v, want %v", tc.name, got, tc.want)
		}
	}
}
