package jobs

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

func (s *JobsService) HandleFlagsAdd(ctx context.Context, e *models.JobEventFlags) error {
	// Tampering check first: verified warmup mail is NOT in the unibox, so we
	// detect it via the warmup_received record. If the recipient marked a
	// warmup email as spam, that harms the pool — penalise the sender for the
	// spam signal AND ban the harmer (they can appeal). Warmup mail isn't
	// tracked in the unibox, so there's nothing else to do for it.
	if s.WarmupRepo != nil {
		if rec, _ := s.WarmupRepo.GetWarmupReceived(ctx, e.EmailID, e.ID); rec != nil {
			switch {
			case s.WarmupService == nil:
			case containsSpamFlag(e.Flags):
				hSender, _ := s.WarmupService.ApplySpamReport(ctx, e.EmailID, rec.SenderAccountID, rec.MessageID, "user_complaint")
				s.markRiskBandFromWarmupHealth(ctx, rec.SenderAccountID, hSender)
				hHarmer, _ := s.WarmupService.RecordTampering(ctx, e.EmailID, rec.MessageID, "spam_flag")
				s.markRiskBandFromWarmupHealth(ctx, e.EmailID, hHarmer)
			case containsTrashFlag(e.Flags) && warmupDeletionCounts(rec, time.Now()):
				// Gmail reports Delete as gaining the TRASH label and only
				// reports the message gone when Trash is emptied, weeks later.
				// The label is the owner's act, so it is judged here, on the
				// same freshness rule as a removal; the later purge is then
				// outside the window and reads as housekeeping.
				hHarmer, _ := s.WarmupService.RecordTampering(ctx, e.EmailID, rec.MessageID, "deletion")
				s.markRiskBandFromWarmupHealth(ctx, e.EmailID, hHarmer)
			}
			return nil
		}
	}

	email, err := s.emailForSyncUpdate(ctx, e.UserID, e.ID, func(message *models.EmailMessageStoreData) {
		for _, flag := range e.Flags {
			if !slices.Contains(message.Flags, flag) {
				message.Flags = append(message.Flags, flag)
			}
		}
		if models.SeenFromFlags(e.Flags) {
			message.Seen = true
		}
		if containsSpamFlag(e.Flags) && message.Folder != models.FolderTrash {
			message.Folder = models.FolderSpam
		}
	})
	if err != nil {
		CaptureError(e.UserID, e.EmailID, fmt.Errorf("Email (%s): %w", e.ID.String(), err))
		return err
	}
	if email == nil {
		return nil
	}

	// Check if a warmup email is being flagged as spam
	if s.WarmupRepo != nil && containsSpamFlag(e.Flags) {
		if tokenStr := warmupTokenFromFlags(email.Flags); tokenStr != "" {
			tokenID, parseErr := uuid.Parse(tokenStr)
			if parseErr == nil {
				token, tokenErr := s.WarmupRepo.FindWarmupToken(ctx, tokenID)
				if tokenErr == nil && token != nil {
					if s.WarmupService != nil {
						health, _ := s.WarmupService.ApplySpamReport(ctx, e.EmailID, token.SenderAccountID, email.MessageID, "user_complaint")
						s.markRiskBandFromWarmupHealth(ctx, token.SenderAccountID, health)
					} else {
						// Degraded mode (no warmup service): record the raw signal
						// so the bands count it whenever they next run. Blocking is
						// owned solely by the banded health model (evaluateMetrics)
						// so every block carries a blocked_until and an appeal path.
						_, _ = s.WarmupRepo.RecordSpamReport(ctx, &repository.SpamReport{
							ID:                uuid.New(),
							ReporterAccountID: e.EmailID,
							ReportedAccountID: token.SenderAccountID,
							MessageID:         email.MessageID,
							ReportType:        "user_complaint",
						})
						s.markRiskBandFromWarmupHealth(ctx, token.SenderAccountID, nil)
					}
				}
			}
		}
	}

	var updated bool

	for i := range e.Flags {
		if !slices.Contains(email.Flags, e.Flags[i]) {
			email.Flags = append(email.Flags, e.Flags[i])
			updated = true
		}
	}

	// Read state is its own column, so gaining \Seen is a change even when the
	// flag array already carried it. Gmail and Graph report read state this
	// way; without this the unibox would only ever be marked read from inside
	// Warmbly, leaving mail the customer read in their own client unread here.
	update := repository.UpdateUniboxEntry{}
	if models.SeenFromFlags(e.Flags) && !email.Seen {
		seen := true
		update.Seen = &seen
		email.Seen = true
		updated = true
	}

	if !updated {
		return nil
	}

	update.Flags = email.Flags
	// A provider-side junking (Gmail SPAM label, IMAP \Junk) moves the
	// message into the spam folder; trash placement is stronger and kept.
	if containsSpamFlag(e.Flags) && email.Folder != models.FolderTrash && email.Folder != models.FolderSpam {
		folder := models.FolderSpam
		update.Folder = &folder
		email.Folder = folder
	}

	if err := s.UniboxRepository.UpdateEntry(
		ctx,
		e.UserID,
		e.EmailID,
		e.ID,
		&update,
	); err != nil {
		return err
	}

	s.publishEmailUpdated(ctx, e.UserID, email)
	return nil
}

func warmupTokenFromFlags(flags []string) string {
	// Try current header name first, then legacy "X-Warmbly-Token" so messages
	// sent before the header rename continue to verify until they age out.
	prefixes := []string{config.WarmupVerifyHeader + ":", "X-Warmbly-Token:"}
	for _, flag := range flags {
		for _, p := range prefixes {
			if strings.HasPrefix(flag, p) {
				return strings.TrimPrefix(flag, p)
			}
		}
	}
	return ""
}

func (s *JobsService) HandleFlagsRemove(ctx context.Context, e *models.JobEventFlags) error {
	email, err := s.emailForSyncUpdate(ctx, e.UserID, e.ID, func(message *models.EmailMessageStoreData) {
		message.Flags = slices.DeleteFunc(message.Flags, func(flag string) bool { return slices.Contains(e.Flags, flag) })
		if models.SeenFromFlags(e.Flags) {
			message.Seen = false
		}
		if message.Folder == models.FolderSpam && !containsSpamFlag(message.Flags) {
			message.Folder = models.FolderInbox
		}
	})
	if err != nil {
		CaptureError(e.UserID, e.EmailID, fmt.Errorf("Email (%s): %w", e.ID.String(), err))
		return err
	}
	if email == nil {
		return nil
	}

	// Losing \Seen is the provider reporting the message back to unread, and
	// that is the column the inbox reads, not the flag array.
	unread := models.SeenFromFlags(e.Flags) && email.Seen

	if len(email.Flags) == 0 && !unread {
		return nil
	}

	// Build a set of flags to remove
	removeSet := make(map[string]struct{}, len(e.Flags))
	for _, f := range e.Flags {
		removeSet[f] = struct{}{}
	}

	// Filter out flags that should be removed
	newFlags := make([]string, 0, len(email.Flags))
	for _, f := range email.Flags {
		if _, toRemove := removeSet[f]; !toRemove {
			newFlags = append(newFlags, f)
		}
	}

	// No change → skip DB update
	if len(newFlags) == len(email.Flags) && !unread {
		return nil
	}

	update := repository.UpdateUniboxEntry{Flags: newFlags}
	if unread {
		seen := false
		update.Seen = &seen
		email.Seen = false
	}
	// Un-junking at the provider (spam label cleared while nothing else
	// still marks it spam) restores the message to the inbox.
	if email.Folder == models.FolderSpam && containsSpamFlag(e.Flags) && !containsSpamFlag(newFlags) {
		folder := models.FolderInbox
		update.Folder = &folder
		email.Folder = folder
	}

	if err := s.UniboxRepository.UpdateEntry(
		ctx,
		e.UserID,
		e.EmailID,
		e.ID,
		&update,
	); err != nil {
		return err
	}

	email.Flags = newFlags
	s.publishEmailUpdated(ctx, e.UserID, email)
	return nil
}

// containsTrashFlag reports the transition Gmail emits for Delete: the TRASH
// label, passed through untranslated by the worker.
func containsTrashFlag(flags []string) bool {
	for _, f := range flags {
		if f == "TRASH" || f == "\\Trash" {
			return true
		}
	}
	return false
}
