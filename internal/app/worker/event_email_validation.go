package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/email"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// probeBudget bounds the two live dials. It starts after the credentials are
// unsealed, so a cold key cache cannot eat the time a slow mail host needs and
// turn a correct password into a refusal. The reply below must be publishable
// after it runs out, so the dials get their own context rather than sharing
// the handler's: a mail host slow enough to use the whole budget used to
// leave no context left to answer on, and the verdict was dropped on the floor.
const probeBudget = 5 * time.Second

// replyBudget is the separate, short budget for publishing the verdict. It is
// derived from the incoming context, not from the probe one, so an exhausted
// probe deadline cannot cancel the answer.
const replyBudget = 3 * time.Second

func (w *WorkerService) HandleEmailValidation(ctx context.Context, data models.EventWorkerEmailValidation) error {
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

	probeCtx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()

	imapDone := make(chan email.ProbeResult, 1)
	smtpDone := make(chan email.ProbeResult, 1)
	// Credentials are untrusted user input. A panic in a bare goroutine cannot
	// be recovered by the caller and would take down the whole worker along
	// with every mailbox assigned to it, so each probe recovers its own.
	probe := func(out chan<- email.ProbeResult, fn func() email.ProbeResult) {
		go func() {
			res := email.ProbeResult{Reason: models.MailProbeProtocol, Detail: "the probe failed unexpectedly"}
			defer func() {
				if r := recover(); r != nil {
					errs.Recover(r)
				}
				out <- res
			}()
			res = fn()
		}()
	}
	imapCreds, smtpCreds := data.Credentials.IMAP, data.Credentials.SMTP
	probe(imapDone, func() email.ProbeResult {
		return email.VerifyImap(probeCtx, imapCreds.Host, imapCreds.Port, imapCreds.Username, imapCreds.Password, imapCreds.Security)
	})
	probe(smtpDone, func() email.ProbeResult {
		return email.VerifySMTP(probeCtx, smtpCreds.Host, smtpCreds.Port, smtpCreds.Username, smtpCreds.Password, smtpCreds.Security)
	})

	verdict := validationVerdict(<-smtpDone, <-imapDone)
	logProbe("smtp", smtpCreds, verdict.SMTP)
	logProbe("imap", imapCreds, verdict.IMAP)

	replyCtx, replyCancel := context.WithTimeout(context.WithoutCancel(ctx), replyBudget)
	defer replyCancel()
	channel := "email_validation:" + data.ProcessID.String()
	// The verdict goes first and the legacy digit after it: a backend that
	// reads verdicts returns on the first message, and one that predates them
	// skips what it cannot parse and takes the digit, so a fleet mid-update
	// keeps answering either way.
	if body, err := json.Marshal(verdict); err == nil {
		if err := w.Cache.Publish(replyCtx, channel, string(body)).Err(); err != nil {
			errs.CaptureException(err)
			return nil
		}
	}
	legacy := "0"
	if verdict.OK {
		legacy = "1"
	}
	if err := w.Cache.Publish(replyCtx, channel, legacy).Err(); err != nil {
		errs.CaptureException(err)
		return nil
	}

	return nil
}

// validationVerdict folds the two probes into the reply the backend reads.
func validationVerdict(smtp, imap email.ProbeResult) models.EmailValidationVerdict {
	return models.EmailValidationVerdict{
		OK:   smtp.OK && imap.OK,
		SMTP: smtp.Leg(),
		IMAP: imap.Leg(),
	}
}

// logProbe records a failed leg with what the server said, and never the
// password: this is the only trace a refused connect leaves on the worker.
func logProbe(leg string, svc *models.Service, res models.EmailValidationLeg) {
	if res.OK {
		return
	}
	log.Info().
		Str("leg", leg).
		Str("host", svc.Host).
		Int("port", svc.Port).
		Str("username", svc.Username).
		Str("reason", res.Reason).
		Str("detail", res.Detail).
		Msg("mailbox credential probe failed")
}
