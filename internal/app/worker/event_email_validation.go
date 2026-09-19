package worker

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/email"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// probeBudget bounds the two live dials. The reply below must be publishable
// after it runs out, so the dials get their own context rather than sharing the
// handler's: a mail host slow enough to use the whole budget used to leave no
// context left to answer on, and the verdict was dropped on the floor.
const probeBudget = 5 * time.Second

// replyBudget is the separate, short budget for publishing the verdict. It is
// derived from the incoming context, not from the probe one, so an exhausted
// probe deadline cannot cancel the answer.
const replyBudget = 3 * time.Second

func (w *WorkerService) HandleEmailValidation(ctx context.Context, data models.EventWorkerEmailValidation) error {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()

	cipher, err := w.CipherService.Cipher(ctx, data.OrgID)
	if err != nil {
		errs.CaptureException(err)
		return nil
	}

	data.Credentials.IMAP.Password, err = cipher.Decrypt(ctx, data.Credentials.IMAP.Password)
	if err != nil {
		errs.CaptureException(err)
		return nil
	}

	data.Credentials.SMTP.Password, err = cipher.Decrypt(ctx, data.Credentials.SMTP.Password)
	if err != nil {
		errs.CaptureException(err)
		return nil
	}

	results := make(chan bool, 2)
	// Credentials are untrusted user input. A panic in a bare goroutine cannot
	// be recovered by the caller and would take down the whole worker along
	// with every mailbox assigned to it, so each probe recovers its own.
	probe := func(fn func() bool) {
		go func() {
			ok := false
			defer func() {
				if r := recover(); r != nil {
					errs.Recover(r)
				}
				results <- ok
			}()
			ok = fn()
		}()
	}
	probe(func() bool {
		return email.VerifyImap(ctx, data.Credentials.IMAP.Host, data.Credentials.IMAP.Port, data.Credentials.IMAP.Username, data.Credentials.IMAP.Password, data.Credentials.IMAP.Security)
	})
	probe(func() bool {
		return email.VerifySMTP(ctx, data.Credentials.SMTP.Host, data.Credentials.SMTP.Port, data.Credentials.SMTP.Username, data.Credentials.SMTP.Password, data.Credentials.SMTP.Security)
	})

	result1 := <-results
	result2 := <-results

	var msg string
	if result1 && result2 {
		msg = "1"
	} else {
		msg = "0"
	}

	replyCtx, replyCancel := context.WithTimeout(context.WithoutCancel(parent), replyBudget)
	defer replyCancel()
	if err := w.Cache.Publish(replyCtx, "email_validation:"+data.ProcessID.String(), msg).Err(); err != nil {
		errs.CaptureException(err)
		return nil
	}

	return nil
}
