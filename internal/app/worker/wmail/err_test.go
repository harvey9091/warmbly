package wmail

import (
	"testing"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

func TestTransientProviderFailuresAreNotReportedAsApplicationErrors(t *testing.T) {
	for _, mailErr := range []*errx.MailError{
		errx.ErrMailServerUnreachable,
		errx.MError(errx.MailErrorWarning, errx.MailErrorCodeConnectionLost, "connection closed", errx.MailErrorResolveMethodRetry),
		errx.ErrMailSendingTooFast,
		errx.ErrMailQuotaExceeded,
	} {
		if reportable(mailErr) {
			t.Errorf("%s was reportable; the worker retries this expected provider condition", mailErr.Code)
		}
	}
}

func TestOnlyWarmblyRateLimitDeactivatesMailbox(t *testing.T) {
	tests := []struct {
		name string
		err  *errx.MailError
		want models.JobEventType
	}{
		{name: "anti abuse limit", err: errx.ErrMailRateLimitExceeded, want: models.JobEventTypeEmailRateLimited},
		{name: "provider throttle", err: errx.ErrMailSendingTooFast, want: models.JobEventTypeEmailFailed},
		{name: "provider quota", err: errx.ErrMailQuotaExceeded, want: models.JobEventTypeEmailFailed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetermineErrorEventType(tc.err); got != tc.want {
				t.Fatalf("event = %s, want %s", got, tc.want)
			}
		})
	}
}
