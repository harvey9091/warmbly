package jobs

import (
	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func CaptureError(userID, emailID uuid.UUID, err error) {
	errs.CaptureException(err,
		errs.Tag("user_id", userID.String()),
		errs.Tag("email_id", emailID.String()),
	)
}
