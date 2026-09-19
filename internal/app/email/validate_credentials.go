package email

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

// ValidateCredentials seals a copy of the credentials with the org DEK and asks
// a worker to try them against the live servers. The caller's credentials are
// never mutated: they go on to be stored under the credentials key, and sealing
// them in place here would double-encrypt the stored password.
func (s *emailService) ValidateCredentials(ctx context.Context, orgID uuid.UUID, workerID string, credentials *models.SmtpImap) *errx.Error {
	processID := uuid.New()

	if credentials == nil || credentials.SMTP == nil || credentials.IMAP == nil {
		return errx.ErrEmailCredentialsRequired
	}

	cipher, err := s.cipherService.Cipher(ctx, orgID)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	sealedIMAP := *credentials.IMAP
	sealedIMAP.Password, err = cipher.Encrypt(ctx, credentials.IMAP.Password)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	sealedSMTP := *credentials.SMTP
	sealedSMTP.Password, err = cipher.Encrypt(ctx, credentials.SMTP.Password)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	// This side must wait longer than the worker's own budget, or the answer
	// arrives after the only listener has given up and every slow mail host
	// reads as an outage.
	subscribeContext, cancel := context.WithTimeout(ctx, validationWait)
	defer cancel()

	// Subscribe before the job goes out. Redis pub/sub keeps nothing for a
	// channel with no subscriber, so a worker that answers between the publish
	// and the SUBSCRIBE lands its reply nowhere and the wait below runs to its
	// deadline with the validation already done.
	r := s.r.Subscribe(subscribeContext, "email_validation:"+processID.String())
	defer r.Close()

	if err := s.publisher.PublishEmailValidation(ctx, workerID, models.EventWorkerEmailValidation{
		OrgID:       orgID,
		ProcessID:   processID,
		Credentials: &models.SmtpImap{SMTP: &sealedSMTP, IMAP: &sealedIMAP},
	}); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	for {
		msg, err := r.ReceiveMessage(subscribeContext)
		if err != nil {
			// Ask the context, not the error, and ask it for the deadline
			// specifically. A deadline reached while waiting is the mail host
			// being slow, not this service being broken, but go-redis pushes
			// the deadline down onto the socket and it comes back as a net
			// timeout rather than context.DeadlineExceeded, so the error's own
			// shape cannot say whose deadline it was. The context can, and it
			// also distinguishes the two ways it ends: only the timeout below
			// is the mail host. A socket timeout while the context is still
			// live is Redis failing, and a caller who went away is neither.
			if errors.Is(subscribeContext.Err(), context.DeadlineExceeded) {
				return errx.ErrEmailValidation
			}
			errs.CaptureException(err)
			return errx.InternalError()
		}

		switch msg.Payload {
		case "1":
			return nil
		case "0":
			return errx.ErrEmailCredentials
		}
	}
}

// validationWait is how long the caller waits for a worker's verdict. It is
// deliberately longer than the worker's own deadline (config.EmailValidationBudget)
// so a verdict produced right at that limit is still heard.
const validationWait = 9 * time.Second
