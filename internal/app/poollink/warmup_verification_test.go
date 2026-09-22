package poollink

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type verificationMailboxRepo struct {
	repository.PoolLinkRepository
	mailbox *models.PoolLinkMailbox
}

func (r verificationMailboxRepo) GetMailboxByRemote(context.Context, uuid.UUID, uuid.UUID) (*models.PoolLinkMailbox, error) {
	return r.mailbox, nil
}

type verificationTokenRepo struct {
	repository.WarmupRepository
	token       *models.WarmupToken
	deliveryErr error
	turns       map[string]bool
	recorded    *[]string
}

func (r verificationTokenRepo) IsWarmupDelivery(context.Context, uuid.UUID, string, string, string) (bool, error) {
	return false, r.deliveryErr
}

func (r verificationTokenRepo) IsWarmupThreadReply(_ context.Context, _ uuid.UUID, parents []string) (bool, error) {
	for _, p := range parents {
		if r.turns[p] {
			return true, nil
		}
	}
	return false, nil
}

func (r verificationTokenRepo) RecordWarmupThreadMessage(_ context.Context, _ uuid.UUID, id string) error {
	*r.recorded = append(*r.recorded, id)
	return nil
}

// A linked instance asks by ancestry for a reply typed by hand in one of the
// cloud's warmup threads; a yes is recorded so the next turn is recognised,
// and a no falls through to the delivery check as before.
func TestVerifyWarmupDeliveryRecognizesThreadRepliesByAncestry(t *testing.T) {
	var recorded []string
	s := &service{
		repo:   verificationMailboxRepo{mailbox: &models.PoolLinkMailbox{EmailAccountID: uuid.New()}},
		warmup: verificationTokenRepo{turns: map[string]bool{"warm-1@cloud.test": true}, recorded: &recorded, deliveryErr: repository.ErrWarmupDeliveryPending},
	}
	inst := &models.PoolLinkInstance{ID: uuid.New()}
	known, err := s.VerifyWarmupDelivery(context.Background(), inst, uuid.New(), models.PoolLinkWarmupDeliveryQuery{MessageID: "<reply-1@gmail.test>", InReplyTo: []string{"warm-1@cloud.test"}})
	if !known || err != nil || len(recorded) != 1 || recorded[0] != "<reply-1@gmail.test>" {
		t.Fatalf("known=%v error=%v recorded=%v; want the reply recognised and remembered", known, err, recorded)
	}
	known, err = s.VerifyWarmupDelivery(context.Background(), inst, uuid.New(), models.PoolLinkWarmupDeliveryQuery{MessageID: "<reply-2@gmail.test>", InReplyTo: []string{"real@prospect.test"}})
	if known || err != errx.ErrServiceDown || len(recorded) != 1 {
		t.Fatalf("known=%v error=%v recorded=%v; want an unknown parent to fall through to the delivery check", known, err, recorded)
	}
}

func (r verificationTokenRepo) FindWarmupToken(context.Context, uuid.UUID) (*models.WarmupToken, error) {
	return r.token, nil
}

func TestVerifyWarmupDeliveryDefersUnconfirmedSenderCopy(t *testing.T) {
	s := &service{
		repo:   verificationMailboxRepo{mailbox: &models.PoolLinkMailbox{EmailAccountID: uuid.New()}},
		warmup: verificationTokenRepo{deliveryErr: repository.ErrWarmupDeliveryPending},
	}
	known, err := s.VerifyWarmupDelivery(context.Background(), &models.PoolLinkInstance{ID: uuid.New()}, uuid.New(), models.PoolLinkWarmupDeliveryQuery{Sender: "sender@test.local", MessageID: "<restamped@test.local>", Subject: "Review"})
	if known || err != errx.ErrServiceDown {
		t.Fatalf("unconfirmed delivery must ask the instance to retry: known=%v error=%v", known, err)
	}
}

func TestVerifyWarmupTokenRecognizesBothMailboxCopies(t *testing.T) {
	sender, recipient := uuid.New(), uuid.New()
	token := &models.WarmupToken{Token: uuid.New(), SenderAccountID: sender, RecipientAccountID: recipient}
	for _, tc := range []struct {
		name    string
		account uuid.UUID
		want    bool
	}{
		{"recipient", recipient, true},
		{"sender Sent copy", sender, true},
		{"unrelated mailbox", uuid.New(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &service{
				repo:   verificationMailboxRepo{mailbox: &models.PoolLinkMailbox{EmailAccountID: tc.account}},
				warmup: verificationTokenRepo{token: token},
			}
			got, err := s.VerifyWarmupToken(context.Background(), &models.PoolLinkInstance{ID: uuid.New()}, uuid.New(), token.Token)
			if err != nil || got != tc.want {
				t.Fatalf("warmup = %v, error = %v; want %v", got, err, tc.want)
			}
		})
	}
}
