package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// HandleEmailAuthError handles authentication errors that require user re-authorization
func (s *JobsService) HandleEmailAuthError(ctx context.Context, event models.EmailErrorEvent) error {
	log.Info().
		Str("email_account_id", event.EmailAccountID).
		Str("error_code", event.ErrorCode).
		Msg("Handling email auth error")

	emailAccountID, err := uuid.Parse(event.EmailAccountID)
	if err != nil {
		log.Error().Err(err).Str("email_account_id", event.EmailAccountID).Msg("Invalid email account ID")
		return err
	}

	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("Invalid user ID")
		return err
	}

	var taskID *uuid.UUID
	if event.TaskID != "" {
		tid, err := uuid.Parse(event.TaskID)
		if err == nil {
			taskID = &tid
		}
	}

	// Store error in database
	if s.EmailAccountErrorRepository != nil {
		errorRecord := &repository.CreateEmailAccountError{
			EmailAccountID: emailAccountID,
			UserID:         userID,
			ErrorCode:      event.ErrorCode,
			Severity:       event.ErrorType,
			ResolveMethod:  event.ResolveMethod,
			Title:          event.UserTitle,
			Message:        event.Message,
			UserMessage:    ptrString(event.UserMessage),
			ActionRequired: ptrString(event.ActionRequired),
			TaskID:         taskID,
		}

		if _, xerr := s.EmailAccountErrorRepository.CreateOnce(ctx, errorRecord); xerr != nil {
			log.Error().Str("error", xerr.Message).Msg("Failed to store email auth error")
		}
	}

	// Mark email account as needing re-auth (set status to inactive)
	s.deactivateAccount(ctx, userID, emailAccountID)

	// Send Pub/Sub notification to user
	if s.StreamingPublisher != nil && event.UserVisible {
		s.StreamingPublisher.PublishEmailError(
			ctx,
			event.UserID,
			emailAccountID,
			uuid.Nil,
			event.UserTitle,
			event.UserMessage,
		)
	}

	return nil
}

// HandleEmailDisabled handles errors indicating the account has been disabled
func (s *JobsService) HandleEmailDisabled(ctx context.Context, event models.EmailErrorEvent) error {
	log.Info().
		Str("email_account_id", event.EmailAccountID).
		Str("error_code", event.ErrorCode).
		Msg("Handling email disabled error")

	emailAccountID, err := uuid.Parse(event.EmailAccountID)
	if err != nil {
		log.Error().Err(err).Str("email_account_id", event.EmailAccountID).Msg("Invalid email account ID")
		return err
	}

	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("Invalid user ID")
		return err
	}

	var taskID *uuid.UUID
	if event.TaskID != "" {
		tid, err := uuid.Parse(event.TaskID)
		if err == nil {
			taskID = &tid
		}
	}

	// Store error in database
	if s.EmailAccountErrorRepository != nil {
		errorRecord := &repository.CreateEmailAccountError{
			EmailAccountID: emailAccountID,
			UserID:         userID,
			ErrorCode:      event.ErrorCode,
			Severity:       event.ErrorType,
			ResolveMethod:  event.ResolveMethod,
			Title:          event.UserTitle,
			Message:        event.Message,
			UserMessage:    ptrString(event.UserMessage),
			ActionRequired: ptrString(event.ActionRequired),
			TaskID:         taskID,
		}

		if _, xerr := s.EmailAccountErrorRepository.CreateOnce(ctx, errorRecord); xerr != nil {
			log.Error().Str("error", xerr.Message).Msg("Failed to store email disabled error")
		}
	}

	// Mark email account as inactive
	s.deactivateAccount(ctx, userID, emailAccountID)

	// Send Pub/Sub notification to user
	if s.StreamingPublisher != nil && event.UserVisible {
		s.StreamingPublisher.PublishEmailError(
			ctx,
			event.UserID,
			emailAccountID,
			uuid.Nil,
			event.UserTitle,
			event.UserMessage,
		)
	}

	return nil
}

// HandleEmailRateLimited handles rate limit exceeded errors (anti-abuse)
func (s *JobsService) HandleEmailRateLimited(ctx context.Context, event models.EmailErrorEvent) error {
	log.Warn().
		Str("email_account_id", event.EmailAccountID).
		Str("error_code", event.ErrorCode).
		Msg("Handling email rate limit exceeded")

	emailAccountID, err := uuid.Parse(event.EmailAccountID)
	if err != nil {
		log.Error().Err(err).Str("email_account_id", event.EmailAccountID).Msg("Invalid email account ID")
		return err
	}

	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("Invalid user ID")
		return err
	}

	var taskID *uuid.UUID
	if event.TaskID != "" {
		tid, err := uuid.Parse(event.TaskID)
		if err == nil {
			taskID = &tid
		}
	}

	// Store error in database
	if s.EmailAccountErrorRepository != nil {
		errorRecord := &repository.CreateEmailAccountError{
			EmailAccountID: emailAccountID,
			UserID:         userID,
			ErrorCode:      event.ErrorCode,
			Severity:       event.ErrorType,
			ResolveMethod:  event.ResolveMethod,
			Title:          event.UserTitle,
			Message:        event.Message,
			UserMessage:    ptrString(event.UserMessage),
			ActionRequired: ptrString(event.ActionRequired),
			TaskID:         taskID,
		}

		if _, xerr := s.EmailAccountErrorRepository.CreateOnce(ctx, errorRecord); xerr != nil {
			log.Error().Str("error", xerr.Message).Msg("Failed to store rate limit error")
		}
	}

	// Mark email account as inactive (terminated due to abuse)
	s.deactivateAccount(ctx, userID, emailAccountID)

	if s.WarmupService != nil {
		health, _ := s.WarmupService.ApplyRateLimitExceeded(ctx, emailAccountID, "worker sync/email rate limit exceeded")
		s.markRiskBandFromWarmupHealth(ctx, emailAccountID, health)
	}

	// Send Pub/Sub notification to user (this is a warning notification)
	if s.StreamingPublisher != nil && event.UserVisible {
		s.StreamingPublisher.PublishEmailWarning(
			ctx,
			event.UserID,
			emailAccountID,
			event.UserTitle,
			event.UserMessage,
		)
	}

	return nil
}

// HandleEmailServerError handles temporary server errors (may auto-resolve)
func (s *JobsService) HandleEmailServerError(ctx context.Context, event models.EmailErrorEvent) error {
	log.Info().
		Str("email_account_id", event.EmailAccountID).
		Str("error_code", event.ErrorCode).
		Msg("Handling email server error")

	emailAccountID, err := uuid.Parse(event.EmailAccountID)
	if err != nil {
		log.Error().Err(err).Str("email_account_id", event.EmailAccountID).Msg("Invalid email account ID")
		return err
	}

	userID, err := uuid.Parse(event.UserID)
	if err != nil {
		log.Error().Err(err).Str("user_id", event.UserID).Msg("Invalid user ID")
		return err
	}

	var taskID *uuid.UUID
	if event.TaskID != "" {
		tid, err := uuid.Parse(event.TaskID)
		if err == nil {
			taskID = &tid
		}
	}

	// Store error in database (as warning, not critical).
	//
	// CreateOnce, not Create: the sync loop retries a refused server about
	// once a minute and relays what it got each time, so the same unresolved
	// failure would otherwise fill the mailbox's error list with a row a
	// minute for as long as the server keeps refusing (issue #405).
	recorded := true
	if s.EmailAccountErrorRepository != nil {
		errorRecord := &repository.CreateEmailAccountError{
			EmailAccountID: emailAccountID,
			UserID:         userID,
			ErrorCode:      event.ErrorCode,
			Severity:       "WARNING",
			ResolveMethod:  event.ResolveMethod,
			Title:          event.UserTitle,
			Message:        event.Message,
			UserMessage:    ptrString(event.UserMessage),
			ActionRequired: ptrString(event.ActionRequired),
			TaskID:         taskID,
		}

		row, xerr := s.EmailAccountErrorRepository.CreateOnce(ctx, errorRecord)
		if xerr != nil {
			log.Error().Str("error", xerr.Message).Msg("Failed to store server error")
		}
		recorded = xerr == nil && row != nil
	}

	// Server errors are temporary - don't change account status
	// The error will be auto-resolved when connectivity is restored

	// Only notify for an error we just recorded: a repeat of one already in
	// the drawer must not toast the reader again every pass.
	if s.StreamingPublisher != nil && event.UserVisible && recorded {
		s.StreamingPublisher.PublishEmailWarning(
			ctx,
			event.UserID,
			emailAccountID,
			event.UserTitle,
			event.UserMessage,
		)
	}

	return nil
}

// ptrString returns a pointer to a string, or nil if empty
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// deactivateAccount marks a mailbox inactive AND tells the worker holding it
// to drop it. The per-worker load filters on status, but that only decides what
// a worker picks up when it starts, and the send path relays an account-level
// failure without stopping the mailbox's own sync loop: without this the worker
// keeps syncing a mailbox that has just been deactivated, failing every pass
// and writing an email_account_errors row each time, until someone restarts it.
func (s *JobsService) deactivateAccount(ctx context.Context, userID, emailAccountID uuid.UUID) {
	if s.EmailRepository == nil {
		return
	}
	inactive := "inactive"
	if _, xerr := s.EmailRepository.Update(ctx, userID.String(), emailAccountID.String(), &models.UpdateEmail{
		Status: &inactive,
	}); xerr != nil {
		log.Error().Str("error", xerr.Message).Msg("Failed to update email account status")
		return
	}
	if s.Publisher == nil {
		return
	}

	// Asked for on its own rather than read off the row Update returned: that
	// row is the mailbox as the dashboard sees it and carries no assignment.
	workerID, xerr := s.EmailRepository.GetWorkerID(ctx, emailAccountID)
	if xerr != nil {
		log.Warn().
			Str("error", xerr.Message).
			Str("email_account_id", emailAccountID.String()).
			Msg("Cannot tell the worker to drop the deactivated mailbox: assignment lookup failed")
		return
	}
	if workerID == nil {
		return
	}
	if err := s.Publisher.PublishRemoveEmail(ctx, *workerID, &models.RemoveWorkerEmail{
		UserID:  userID.String(),
		EmailID: emailAccountID.String(),
	}); err != nil {
		log.Warn().Err(err).
			Str("email_account_id", emailAccountID.String()).
			Str("worker_id", workerID.String()).
			Msg("Failed to tell the worker to drop the deactivated mailbox")
	}
}
