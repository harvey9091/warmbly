package worker

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/app/worker/wmail"
	"github.com/warmbly/warmbly/internal/client/smtpimap/imap"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/models"
)

// HandleWarmupAction executes recipient-side warmup actions on the mailbox.
//
// The worker just runs whatever actions it receives, immediately. The
// recipient-side "dwell" and the immediate-vs-delayed split are now owned by the
// CONSUMER's durable schedule (internal/app/consumer/warmup_engagement_poller):
// the consumer publishes the immediate leg (folder + spam-rescue) right away and
// the delayed leg (read / important / star) when its fire_at passes, each with
// DelaySeconds=0. That makes the dwell survive a worker restart, which the old
// in-process time.AfterFunc here could not.
func (w *WorkerService) HandleWarmupAction(ctx context.Context, action models.WarmupEmailAction) error {
	log.Info().
		Str("email_id", action.EmailID.String()).
		Str("gmail_id", action.GmailID).
		Uint32("uid", action.UID).
		Uint32("mailbox_uid_validity", action.MailboxUIDValidity).
		Strs("actions", action.Actions).
		Msg("Processing warmup email action")

	if len(action.Actions) == 0 {
		return nil
	}

	w.mailManager.RLock()
	mail, exists := w.mailManager.Emails[action.EmailID]
	w.mailManager.RUnlock()
	if !exists {
		log.Warn().Str("email_id", action.EmailID.String()).Msg("Email account not found for warmup action")
		return nil
	}

	switch {
	case mail.GoogleData != nil && mail.GoogleData.Client != nil:
		w.runGoogleWarmupActions(ctx, mail, action)
	case mail.GraphData != nil && mail.GraphData.Client != nil:
		w.runGraphWarmupActions(ctx, mail, action)
	case mail.SmtpImapData != nil && mail.SmtpImapData.ImapClient != nil:
		w.runImapWarmupActions(ctx, mail, action)
	default:
		log.Warn().
			Str("email_id", action.EmailID.String()).
			Msg("No mail client available for warmup actions; skipping")
	}
	return nil
}

func (w *WorkerService) runGoogleWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	placement, folder := warmupFiling(action)
	// The archive placement wants the message out of every view without a label
	// of its own, which on Gmail is "remove INBOX, add nothing".
	label := folder
	if placement == models.WarmupPlacementArchive {
		label = ""
	}

	for _, act := range action.Actions {
		switch act {
		case models.WarmupActionFile:
			if err := mail.GoogleData.Client.FileWarmup(ctx, action.GmailID, label); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Str("folder", label).Msg("Failed to file warmup message (Gmail)")
			}
		case models.WarmupActionMarkRead:
			if err := mail.GoogleData.Client.MarkAsRead(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to mark as read")
			}
		case models.WarmupActionRescueFromSpam:
			toInbox := placement == models.WarmupPlacementInbox
			if err := mail.GoogleData.Client.RescueFromSpam(ctx, action.GmailID, label, toInbox); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to rescue from spam (Gmail)")
			}
		case models.WarmupActionMarkImportant:
			if err := mail.GoogleData.Client.MarkImportant(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to mark important")
			}
		case models.WarmupActionStar:
			if err := mail.GoogleData.Client.AddStar(ctx, action.GmailID); err != nil {
				log.Error().Err(err).Str("gmail_id", action.GmailID).Msg("Failed to star warmup message")
			}
		default:
			log.Warn().Str("action", act).Msg("Unknown warmup action")
		}
	}
}

// runGraphWarmupActions applies recipient-side warmup engagement to a Microsoft
// Graph mailbox. action.GmailID carries the Graph message id (the provider id
// field is provider-agnostic). Star maps to the follow-up flag, the closest
// Outlook equivalent of a Gmail star.
func (w *WorkerService) runGraphWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	client := mail.GraphData.Client

	// A Graph message id changes whenever the message is moved (copy+delete), so
	// resolve the live id from the immutable RFC Message-ID before acting. This
	// leg may run after an earlier leg already moved the message to Warmbly.
	msgID := action.GmailID
	if action.RFCMessageID != "" {
		if resolved, err := client.ResolveMessageID(ctx, action.RFCMessageID); err == nil && resolved != "" {
			msgID = resolved
		}
	}

	placement, folder := warmupFiling(action)

	for _, act := range action.Actions {
		switch act {
		case models.WarmupActionFile:
			// Out of Junk into the destination is one move, and on Exchange it
			// is also the "not junk" signal, so the rescue below finds nothing
			// left in Junk and correctly does nothing.
			var newID string
			var err error
			if placement == models.WarmupPlacementArchive {
				newID, err = client.MoveToArchive(ctx, msgID)
			} else {
				newID, err = client.MoveToFolder(ctx, msgID, folder)
			}
			if err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Str("folder", folder).Msg("Failed to file warmup message (Graph)")
				continue
			}
			if newID != "" {
				msgID = newID // subsequent actions target the moved copy
			}
		case models.WarmupActionMarkRead:
			if err := client.MarkAsRead(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to mark as read (Graph)")
			}
		case models.WarmupActionRescueFromSpam:
			newID, err := client.RemoveFromSpam(ctx, msgID)
			if err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to rescue from junk (Graph)")
				continue
			}
			if newID != "" {
				msgID = newID
			}
		case models.WarmupActionMarkImportant:
			if err := client.MarkImportant(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to mark important (Graph)")
			}
		case models.WarmupActionStar:
			if err := client.AddFlag(ctx, msgID); err != nil {
				log.Error().Err(err).Str("graph_id", msgID).Msg("Failed to flag warmup message (Graph)")
			}
		default:
			log.Warn().Str("action", act).Msg("Unknown warmup action")
		}
	}
}

func (w *WorkerService) runImapWarmupActions(ctx context.Context, mail *wmail.WMail, action models.WarmupEmailAction) {
	boxes := mail.SmtpImapData.Mailboxes
	imapClient := mail.SmtpImapData.ImapClient
	sourceBox := lookupWarmupSourceFolder(boxes, action)

	inboxName := "INBOX"
	if inboxBox := lookupInbox(boxes); inboxBox != nil {
		inboxName = inboxBox.Name
	}

	// dst is where the filing action puts this mailbox's warmup mail, and is
	// empty for the placement that leaves it where the provider put it.
	placement, folder := warmupFiling(action)
	dst := ""
	switch placement {
	case models.WarmupPlacementFolder:
		dst = folder
	case models.WarmupPlacementArchive:
		if box := lookupArchive(boxes); box != nil {
			dst = box.Name
		} else {
			// No archive on this server. The warmup folder is the destination
			// that always exists, because we create it.
			dst = folder
			log.Debug().Str("email_id", action.EmailID.String()).Msg("No archive folder on this account; filing warmup in its own folder")
		}
	}

	sentName := ""
	if sentBox := lookupSent(boxes); sentBox != nil {
		sentName = sentBox.Name
	}

	files := hasWarmupAction(action.Actions, models.WarmupActionFile)
	boxName, uid := w.locateWarmupMessage(ctx, imapClient, action, sourceBox, files, dst, inboxName, sentName)
	if boxName == "" {
		log.Warn().
			Str("folder", action.MailboxFolder).
			Uint32("uid_validity", action.MailboxUIDValidity).
			Str("email_id", action.EmailID.String()).
			Msg("Warmup action skipped: the message could not be located in any folder")
		return
	}

	// moved is set once the message leaves boxName. Every later action would
	// address a UID that folder no longer has, and IMAP silently ignores a
	// STORE on a UID that is gone, so they are skipped rather than aimed at
	// whatever message inherits the number.
	moved := false

	for _, act := range action.Actions {
		if moved {
			continue
		}
		switch act {
		case models.WarmupActionFile:
			if dst == "" {
				continue
			}
			// A message already in the destination is not moved, and its UID is
			// still good for the actions after this one.
			did, err := imapClient.MoveToFolder(ctx, boxName, dst, uid)
			if err != nil {
				log.Error().Err(err).Uint32("uid", uid).Str("folder", dst).Msg("Failed to file warmup message (IMAP)")
				continue
			}
			moved = did
		case models.WarmupActionMarkRead:
			if err := imapClient.MarkAsRead(ctx, boxName, uid); err != nil {
				log.Error().Err(err).Uint32("uid", uid).Msg("Failed to mark as read (IMAP)")
			}
		case models.WarmupActionRescueFromSpam:
			// Only a message still sitting in a Junk folder is rescued. With any
			// placement but "inbox" the filing move above already took it out of
			// Junk, which is itself the not-spam signal the server learns from.
			if sourceBox == nil || boxName != sourceBox.Name || !imap.IsSpamMailbox(sourceBox.Name, sourceBox.Attrs) {
				continue
			}
			if err := imapClient.RemoveFromSpam(ctx, boxName, inboxName, uid); err != nil {
				log.Error().Err(err).Uint32("uid", uid).Msg("Failed to remove from spam (IMAP)")
				continue
			}
			moved = true
		case models.WarmupActionMarkImportant:
			if err := imapClient.MarkImportant(ctx, boxName, uid); err != nil {
				log.Error().Err(err).Uint32("uid", uid).Msg("Failed to mark important (IMAP)")
			}
		case models.WarmupActionStar:
			// No-op on IMAP: \Flagged is already set by mark_important, so
			// starring here would just re-flag the same message. Star is a
			// Gmail-only distinct signal.
			continue
		default:
			log.Warn().Str("action", act).Msg("Unknown warmup action")
		}
	}
}

// locateWarmupMessage resolves the folder and UID an action should act on.
//
// The arrival folder and UID travel with the action, and for the leg that files
// the message they are still correct. The delayed leg (read / important) is a
// separate event published after the filing already moved the message, so its
// UID addresses nothing in the folder the mail arrived in. IMAP answers a STORE
// on a missing UID with silence, which is why that leg had been doing nothing
// at all on every mailbox that files its warmup: the mail stayed unread, and an
// unread count on the folder is the thing its owner was not supposed to notice.
//
// The Message-ID is the one identifier a move does not change, so the likely
// destinations are searched for it, most likely first. An empty folder name
// means the message is nowhere we know to look.
func (w *WorkerService) locateWarmupMessage(
	ctx context.Context,
	client warmupIMAPClient,
	action models.WarmupEmailAction,
	sourceBox *models.Mailbox,
	files bool,
	dst, inboxName, sentName string,
) (string, uint32) {
	if files && sourceBox != nil {
		return sourceBox.Name, action.UID
	}
	if action.RFCMessageID != "" {
		// Ordered by likelihood: the destination a previous leg filed it into,
		// then the inbox a rescue put it back in, then Sent for our own copy of
		// a warmup send, then the folder the event says it arrived in.
		candidates := []string{dst, inboxName, sentName}
		if sourceBox != nil {
			candidates = append(candidates, sourceBox.Name)
		}
		seen := make(map[string]bool, len(candidates))
		for _, name := range candidates {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			uid, err := client.FindUIDByMessageID(ctx, name, action.RFCMessageID)
			if err != nil {
				log.Debug().Err(err).Str("folder", name).Str("email_id", action.EmailID.String()).Msg("Could not search a folder for the warmup message")
				continue
			}
			if uid != 0 {
				return name, uid
			}
		}
	}
	if sourceBox != nil {
		return sourceBox.Name, action.UID
	}
	return "", 0
}

// warmupIMAPClient is the slice of the IMAP client the warmup actions use, so
// locateWarmupMessage can be exercised without a server.
type warmupIMAPClient interface {
	FindUIDByMessageID(ctx context.Context, mailboxName, rfcMessageID string) (uint32, error)
}

// hasWarmupAction reports whether the leg contains a given warmup action.
func hasWarmupAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

// lookupWarmupSourceFolder resolves the folder an action's UID lives in.
//
// The folder is found by name, its identity. The UIDVALIDITY still has to
// match: it is the generation the stored UID belongs to, and a server that
// reissued it has given that number to some other message, so acting on it
// would star or file a message nobody asked about. Nothing to act on is the
// right answer there.
//
// An action published before the folder name was carried has only the
// UIDVALIDITY to go on, which is the old behaviour and stays as the fallback.
func lookupWarmupSourceFolder(boxes []*models.Mailbox, action models.WarmupEmailAction) *models.Mailbox {
	if action.MailboxFolder == "" {
		return lookupMailboxByUIDValidity(boxes, action.MailboxUIDValidity)
	}
	for _, b := range boxes {
		if b != nil && b.Name == action.MailboxFolder && b.UIDValidity == action.MailboxUIDValidity {
			return b
		}
	}
	return nil
}

func lookupMailboxByUIDValidity(boxes []*models.Mailbox, uidValidity uint32) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && b.UIDValidity == uidValidity {
			return b
		}
	}
	return nil
}

func lookupInbox(boxes []*models.Mailbox) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && imap.IsInboxMailbox(b.Name, b.Attrs) {
			return b
		}
	}
	return nil
}

func lookupSent(boxes []*models.Mailbox) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && imap.IsSentMailbox(b.Name, b.Attrs) {
			return b
		}
	}
	return nil
}

func lookupArchive(boxes []*models.Mailbox) *models.Mailbox {
	for _, b := range boxes {
		if b != nil && imap.IsArchiveMailbox(b.Name, b.Attrs) {
			return b
		}
	}
	return nil
}

// warmupFiling resolves where the filing action should put the message, from
// what the control plane sent with the action. An event published before these
// fields existed carries neither, and the default folder is exactly what every
// mailbox did then, so an in-flight event behaves the same either way.
func warmupFiling(action models.WarmupEmailAction) (placement, folder string) {
	placement = action.Placement
	if !models.ValidWarmupPlacement(placement) {
		placement = models.WarmupPlacementFolder
	}
	folder = strings.TrimSpace(action.TargetFolder)
	if folder == "" {
		folder = config.WarmupFolderDefault
	}
	return placement, folder
}
