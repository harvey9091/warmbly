// Package adminoutreach is the platform-mailer composer: an admin types
// a message, picks a recipient, and Warmbly sends from its noreply
// address with a configurable Reply-To so replies route to a real
// inbox. Distinct from the campaign emailsend path (which sends
// through customer mailboxes) so the two abuse surfaces never share
// code paths.
package adminoutreach

import (
	"context"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/notify"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/repository"
)

type Service interface {
	Send(ctx context.Context, adminID uuid.UUID, req *models.SendAdminOutreachRequest) (*models.AdminOutreachMessage, *errx.Error)
	Search(ctx context.Context, search *models.AdminOutreachSearch) (*models.AdminOutreachResult, *errx.Error)
}

type service struct {
	repo     repository.AdminOutreachRepository
	userRepo repository.UserRepository
	orgRepo  repository.OrganizationRepository
	mailer   notify.EmailNotificationService
}

func NewService(
	repo repository.AdminOutreachRepository,
	userRepo repository.UserRepository,
	orgRepo repository.OrganizationRepository,
	mailer notify.EmailNotificationService,
) Service {
	return &service{repo: repo, userRepo: userRepo, orgRepo: orgRepo, mailer: mailer}
}

// Send resolves the recipient address from the request, persists the
// queued row, performs the SMTP/SES send, and marks the row sent or
// failed atomically with respect to the audit log.
func (s *service) Send(ctx context.Context, adminID uuid.UUID, req *models.SendAdminOutreachRequest) (*models.AdminOutreachMessage, *errx.Error) {
	to, xerr := s.resolveRecipient(ctx, req)
	if xerr != nil {
		return nil, xerr
	}

	var replyToPtr *string
	if req.ReplyTo != "" {
		replyToPtr = &req.ReplyTo
	}

	m := &models.AdminOutreachMessage{
		ID:       uuid.New(),
		SentBy:   adminID,
		ToUserID: req.ToUserID,
		ToOrgID:  req.ToOrgID,
		ToEmail:  to,
		ReplyTo:  replyToPtr,
		Subject:  req.Subject,
		Body:     req.Body,
		Status:   models.AdminOutreachStatusQueued,
	}

	if err := s.repo.Insert(ctx, m); err != nil {
		errs.CaptureException(err)
		return nil, errx.New(errx.Internal, "failed to record outreach")
	}

	if err := s.mailer.SendOutreach(ctx, []string{to}, req.ReplyTo, req.Subject, req.Body); err != nil {
		errs.CaptureException(err)
		_ = s.repo.MarkFailed(ctx, m.ID, err.Error())
		errStr := err.Error()
		m.Status = models.AdminOutreachStatusFailed
		m.Error = &errStr
		return m, errx.New(errx.Internal, "send failed: "+err.Error())
	}

	if err := s.repo.MarkSent(ctx, m.ID); err != nil {
		// Mail went out; audit row stuck in queued. Log and return
		// success so the admin isn't confused into re-sending.
		errs.CaptureException(err)
	}
	m.Status = models.AdminOutreachStatusSent
	return m, nil
}

func (s *service) Search(ctx context.Context, search *models.AdminOutreachSearch) (*models.AdminOutreachResult, *errx.Error) {
	result, err := s.repo.Search(ctx, search)
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.New(errx.Internal, "failed to load outreach log")
	}
	return result, nil
}

func (s *service) resolveRecipient(ctx context.Context, req *models.SendAdminOutreachRequest) (string, *errx.Error) {
	set := 0
	if req.ToUserID != nil {
		set++
	}
	if req.ToOrgID != nil {
		set++
	}
	if req.ToEmail != "" {
		set++
	}
	if set != 1 {
		return "", errx.New(errx.BadRequest, "exactly one of to_user_id, to_org_id, to_email is required")
	}

	if req.ToEmail != "" {
		return req.ToEmail, nil
	}
	if req.ToUserID != nil {
		u, err := s.userRepo.GetUser(ctx, *req.ToUserID)
		if err != nil {
			errs.CaptureException(err)
			return "", errx.New(errx.Internal, "failed to load user")
		}
		if u == nil {
			return "", errx.New(errx.NotFound, "user not found")
		}
		return u.Email, nil
	}
	// to_org_id → owner's email
	org, err := s.orgRepo.GetByID(ctx, *req.ToOrgID)
	if err != nil {
		errs.CaptureException(err)
		return "", errx.New(errx.Internal, "failed to load organization")
	}
	if org == nil {
		return "", errx.New(errx.NotFound, "organization not found")
	}
	u, err := s.userRepo.GetUser(ctx, org.OwnerUserID)
	if err != nil {
		errs.CaptureException(err)
		return "", errx.New(errx.Internal, "failed to load owner")
	}
	if u == nil {
		return "", errx.New(errx.NotFound, "organization owner not found")
	}
	return u.Email, nil
}
