package wmail

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/client/smtpimap/imap"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Sync is one IMAP pass: follow every folder's CONDSTORE mod-sequence for
// live changes, then advance the backfill under its budget.
//
// A folder seen for the first time is baselined, not walked: its current
// HIGHESTMODSEQ is recorded so live sync starts from now, and its history is
// left to the backfill, which imports newest first under the policy's window
// and cap. That replaces the old first sight, which fetched every message in
// every folder oldest first and ran straight into the rate limiter.
func (w *WMail) Sync(ctx context.Context) *errx.MailError {
	if w.SmtpImapData == nil || w.SmtpImapData.ImapClient == nil {
		return nil
	}
	w.beginTick()
	stats := &tickStats{}

	client := w.SmtpImapData.ImapClient
	// A mailbox left selected by the previous pass freezes LIST-STATUS on this
	// connection, so release it before asking what changed.
	client.ReleaseMailbox()
	folders, err := client.Folders()
	if err != nil {
		return err
	}
	w.reportFolderOverflow()
	// Folders() already drops Gmail's label views. Dropping them here too
	// costs nothing and keeps the pass correct against any listing: a view
	// that reached it would re-file known mail as archive under a second UID.
	folders = slices.DeleteFunc(folders, func(b models.Mailbox) bool { return imapVirtualFolder(&b) })

	// condStore decides the incremental strategy for the whole account:
	// mod-sequences where the server has CONDSTORE, UIDNEXT where it does not
	// (Outlook.com, Microsoft 365 over IMAP, Yahoo, many hosted servers).
	condStore := client.HasCondStore()

	for i := range folders {
		box := &folders[i]
		befBox := w.SmtpImapData.FindPair(box)
		if befBox == nil {
			// First sight: baseline. Live sync starts from this cursor; the
			// backfill owns everything before it.
			saved := *box
			if err := w.mboxEvent(&saved); err != nil {
				return nil
			}
			w.SmtpImapData.Mailboxes = append(w.SmtpImapData.Mailboxes, &saved)
			continue
		}

		changed := imapFolderChanged(befBox, box, condStore)
		fullyProcessed := true
		if changed && !stats.aborted {
			w.SmtpImapData.mailbox = box.UIDValidity
			w.SmtpImapData.folder = imapCanonicalFolder(box)
			done, err := w.imapIncremental(ctx, box, befBox, condStore, stats)
			if err != nil {
				return err
			}
			fullyProcessed = done
		} else if changed {
			// The pass was aborted before this folder; hold its cursor too.
			fullyProcessed = false
		}

		if changed || befBox.Name != box.Name || !slices.Equal(befBox.Attrs, box.Attrs) {
			// The stored cursor only moves once every change up to it was
			// stored; a deferred message keeps the folder re-asked.
			next := *box
			if !fullyProcessed {
				next.HighestModSeq = befBox.HighestModSeq
				next.UIDNext = befBox.UIDNext
			}
			if err := w.mboxEvent(&next); err != nil {
				return nil
			}
			for _, ibox := range w.SmtpImapData.Mailboxes {
				if ibox.UIDValidity == box.UIDValidity {
					ibox.HighestModSeq = next.HighestModSeq
					ibox.UIDNext = next.UIDNext
					ibox.Name = next.Name
					ibox.Attrs = next.Attrs
				}
			}
		}

		// Without CONDSTORE a message marked read elsewhere moves no cursor,
		// so read state is mirrored by a periodic scan instead. It runs after
		// the arrivals above so a message stored this pass is already known.
		if !condStore && !stats.aborted {
			w.SmtpImapData.mailbox = box.UIDValidity
			w.SmtpImapData.folder = imapCanonicalFolder(box)
			if _, err := w.SmtpImapData.ImapClient.SelectForSync(box.Name); err != nil {
				return err
			}
			if err := w.imapScanFlags(ctx, box, stats); err != nil {
				return err
			}
		}
	}

	// Collect deletions first to avoid modifying the slice during iteration
	var deleted []uint32
outer:
	for _, box := range w.SmtpImapData.Mailboxes {
		for _, f := range folders {
			if box.UIDValidity == f.UIDValidity {
				continue outer
			}
		}

		if err := w.onEvent(models.JobEventTypeMailboxDelete, &models.JobEventMailboxDelete{
			UserID:      w.UserID,
			EmailID:     w.ID,
			UIDValidity: box.UIDValidity,
		}); err != nil {
			return nil
		}
		deleted = append(deleted, box.UIDValidity)
	}

	if len(deleted) > 0 {
		for _, uidv := range deleted {
			delete(w.flagScan, uidv)
		}
		filtered := w.SmtpImapData.Mailboxes[:0]
		for _, b := range w.SmtpImapData.Mailboxes {
			if !slices.Contains(deleted, b.UIDValidity) {
				filtered = append(filtered, b)
			}
		}
		w.SmtpImapData.Mailboxes = filtered
	}

	if !stats.aborted {
		if err := w.imapBackfill(ctx, folders, stats); err != nil {
			return err
		}
	}

	w.endTick(stats)
	return nil
}

// imapFolderChanged reports whether a folder has anything new since the
// cursor we hold for it. With CONDSTORE the mod-sequence answers for new mail
// AND flag changes; without it only arrivals are visible here, and flag
// changes are picked up by the periodic scan in imapIncremental.
func imapFolderChanged(before, now *models.Mailbox, condStore bool) bool {
	if condStore {
		return before.HighestModSeq != now.HighestModSeq
	}
	return before.UIDNext != now.UIDNext
}

// imapIncremental stores what changed in one folder since the held cursor.
// Known messages relay their flags unbudgeted; new ones are admitted newest
// first. It reports whether every change was stored, which is what lets the
// folder's cursor advance.
func (w *WMail) imapIncremental(ctx context.Context, box, before *models.Mailbox, condStore bool, stats *tickStats) (bool, *errx.MailError) {
	client := w.SmtpImapData.ImapClient
	count, err := client.SelectForSync(box.Name)
	if err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	var uids []goimap.UID
	if condStore {
		uids, err = client.SearchChangedSince(before.HighestModSeq)
	} else {
		uids, err = client.SearchNewSince(before.UIDNext)
	}
	if err != nil {
		return false, err
	}
	if len(uids) == 0 {
		return true, nil
	}
	// Newest first: when budget is short, the freshest mail lands first.
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })

	for lo := 0; lo < len(uids); lo += config.ImapFetchBatchSize {
		hi := min(lo+config.ImapFetchBatchSize, len(uids))
		fetched, err := client.FetchEnvelopes(ctx, uids[lo:hi])
		if err != nil {
			return false, err
		}
		done, err := w.imapApply(ctx, fetched, false, stats)
		if err != nil {
			return false, err
		}
		// A denied lane means no later batch can be stored either, and every
		// extra batch still counts new mail toward flood detection: a mailbox
		// unfrozen on a long backlog would deactivate itself walking mail it
		// cannot keep. Stop here; the held mod-sequence re-offers the rest.
		if !done || stats.aborted || ctx.Err() != nil {
			return false, nil
		}
	}
	return true, nil
}

// imapApply routes one fetched batch: known messages get an UPDATE_EMAIL,
// unknown ones are stored if their lane admits them. backfill selects the
// backfill lane and skips flood accounting. Returns whether every unknown
// message in the batch was stored.
func (w *WMail) imapApply(ctx context.Context, fetched []*imap.Fetched, backfill bool, stats *tickStats) (bool, *errx.MailError) {
	sort.Slice(fetched, func(i, j int) bool { return fetched[i].Email.UID > fetched[j].Email.UID })

	var fresh []*imap.Fetched
	for _, f := range fetched {
		internal, err := w.EmailMessageMapRepository.Get(ctx, w.UserID, w.ID, f.Email.MessageID)
		if err != nil {
			return false, w.controlPlaneError(err, stats)
		}
		if internal == nil {
			fresh = append(fresh, f)
			continue
		}
		if backfill {
			// The backfill only cares about what it has not stored.
			continue
		}
		internalID, perr := uuid.Parse(internal.ID)
		if perr != nil {
			continue
		}
		if err := w.onEvent(models.JobEventTypeEmailUpdate, &models.JobEventEmailUpdate{
			UserID:  w.UserID,
			EmailID: w.ID,
			ID:      internalID,
			UID:     f.Email.UID,
			ModSeq:  f.Email.ModSeq,
			Mailbox: w.SmtpImapData.mailbox,
			Folder:  w.SmtpImapData.folder,
			Flags:   f.Email.Flags,
		}); err != nil {
			return false, w.controlPlaneError(err, stats)
		}
	}

	if !backfill && len(fresh) > 0 {
		ids := make([]string, 0, len(fresh))
		for _, f := range fresh {
			ids = append(ids, f.Email.MessageID)
		}
		if w.observeLive(ctx, ids, stats) {
			return false, nil
		}
	}

	policy := w.gov.Policy()
	all := true
	for _, f := range fresh {
		if stats.aborted {
			return false, nil
		}
		if backfill && w.tracker.state.BackfillSynced >= policy.BackfillMessages {
			return false, nil
		}
		if !w.admit(ctx, w.laneOf(ctx, f.Email.MessageID, f.Email, backfill), stats) {
			all = false
			if backfill {
				return false, nil
			}
			continue
		}
		w.SmtpImapData.ImapClient.FetchBody(f)
		if err := w.imapStore(ctx, f.Email); err != nil {
			return false, w.controlPlaneError(err, stats)
		}
		w.laneCache.forget(f.Email.MessageID)
		if backfill {
			w.tracker.state.BackfillSynced++
			w.tracker.mark()
			w.tracker.setFolder(strconv.FormatUint(uint64(w.SmtpImapData.mailbox), 10), models.SyncFolderCursor{UID: f.Email.UID})
		}
	}
	return all, nil
}

// imapStore threads a new message and hands it to storeNew.
func (w *WMail) imapStore(ctx context.Context, msg *models.EmailMessageData) error {
	msg.ID = uuid.New()
	now := time.Now()

	var threadID string
	var parentID string
	if len(msg.InReplyTo) > 0 {
		parentID = msg.InReplyTo[len(msg.InReplyTo)-1]
	} else if len(msg.ReplyTo) > 0 {
		parentID = msg.ReplyTo[len(msg.ReplyTo)-1]
	}

	if parentID != "" {
		internalParent, _ := w.EmailMessageMapRepository.Get(ctx, w.UserID, w.ID, parentID)
		if internalParent != nil {
			// Join the parent's thread; using the parent's internal id here
			// forked a new thread at every reply depth.
			threadID = internalParent.ThreadID
			if threadID == "" {
				threadID = parentID
			}
		} else {
			// Parent unknown (e.g. reply to a pre-connect message): root a
			// thread on the parent's RFC id so siblings still group.
			threadID = parentID
		}
	} else {
		threadID = msg.MessageID
	}

	data := &models.EmailMessageStoreData{
		ID:           msg.ID,
		EmailID:      w.ID,
		Mailbox:      w.SmtpImapData.mailbox,
		Folder:       w.SmtpImapData.folder,
		ThreadID:     threadID,
		MessageID:    msg.MessageID,
		GmailID:      msg.GmailID,
		ParentID:     parentID,
		UID:          msg.UID,
		ModSeq:       msg.ModSeq,
		Flags:        msg.Flags,
		BCC:          msg.BCC,
		CC:           msg.CC,
		FromAddr:     msg.From,
		InReplyTo:    msg.InReplyTo,
		ReplyTo:      msg.ReplyTo,
		ToAddr:       msg.To,
		Subject:      msg.Subject,
		Size:         msg.Size,
		InternalDate: msg.InternalDate,
		SentDate:     msg.Date,
		Snippet:      GenerateSnippet(msg.BodyPlain, msg.BodyHTML),
		BodyText:     SearchText(msg.BodyPlain, msg.BodyHTML),
		Seen:         false,
		UpdatedAt:    now,
		CreatedAt:    now,
	}
	return w.storeNew(ctx, msg, data, msg.MessageID)
}

// imapBackfill advances the initial import: for every eligible folder, walk
// the UIDs inside the window newest first, from the saved floor downward,
// until the pacing budget or the cap says stop. Progress is relayed after
// every message, so a replaced worker resumes rather than restarts.
func (w *WMail) imapBackfill(ctx context.Context, folders []models.Mailbox, stats *tickStats) *errx.MailError {
	st := &w.tracker.state
	if st.BackfillStatus == models.SyncBackfillComplete {
		return nil
	}
	policy := w.gov.Policy()
	w.tracker.startBackfill(time.Now(), policy.BackfillDays)
	since := *st.BackfillSince
	client := w.SmtpImapData.ImapClient

	allDone := true
	for i := range folders {
		box := &folders[i]
		if !imapBackfillEligible(box) {
			continue
		}
		if stats.aborted || stats.laneDenied(LaneBackfill) {
			return nil
		}
		key := strconv.FormatUint(uint64(box.UIDValidity), 10)
		cur := w.tracker.folder(key)
		if cur.Done {
			continue
		}
		if st.BackfillSynced >= policy.BackfillMessages {
			w.tracker.completeBackfill(time.Now())
			return nil
		}
		w.SmtpImapData.mailbox = box.UIDValidity
		w.SmtpImapData.folder = imapCanonicalFolder(box)

		count, err := client.SelectForSync(box.Name)
		if err != nil {
			return err
		}
		if count == 0 {
			w.tracker.setFolder(key, models.SyncFolderCursor{Done: true})
			continue
		}
		uids, err := client.SearchSince(since)
		if err != nil {
			return err
		}
		remaining := uids[:0:0]
		for _, uid := range uids {
			if cur.UID == 0 || uid < goimap.UID(cur.UID) {
				remaining = append(remaining, uid)
			}
		}
		if len(remaining) == 0 {
			w.tracker.setFolder(key, models.SyncFolderCursor{UID: cur.UID, Done: true})
			continue
		}
		allDone = false
		sort.Slice(remaining, func(i, j int) bool { return remaining[i] > remaining[j] })

		for lo := 0; lo < len(remaining); lo += config.ImapFetchBatchSize {
			hi := min(lo+config.ImapFetchBatchSize, len(remaining))
			fetched, err := client.FetchEnvelopes(ctx, remaining[lo:hi])
			if err != nil {
				return err
			}
			done, err := w.imapApply(ctx, fetched, true, stats)
			if err != nil {
				return err
			}
			if st.BackfillSynced >= policy.BackfillMessages {
				w.tracker.completeBackfill(time.Now())
				return nil
			}
			if !done || ctx.Err() != nil {
				// Pacing stopped us mid-folder; the floor is already at the last
				// stored UID and the next tick continues below it.
				return nil
			}
			// A batch that was entirely known still moves the floor.
			w.tracker.setFolder(key, models.SyncFolderCursor{UID: uint32(remaining[hi-1])})
		}
		w.tracker.setFolder(key, models.SyncFolderCursor{UID: uint32(remaining[len(remaining)-1]), Done: true})
	}

	if allDone {
		w.tracker.completeBackfill(time.Now())
	}
	return nil
}

// imapVirtualFolder, imapBackfillEligible and imapCanonicalFolder classify a
// folder. The rules live in the imap client package, next to the LIST that
// produces the attributes, so the sync loop and the Sent-folder resolver
// cannot drift apart.
func imapVirtualFolder(box *models.Mailbox) bool {
	return imap.IsVirtualFolder(*box)
}

func imapBackfillEligible(box *models.Mailbox) bool {
	return imap.BackfillEligible(*box)
}

func imapCanonicalFolder(box *models.Mailbox) string {
	return imap.CanonicalFolder(*box)
}

// controlPlaneError handles a failed map lookup, body store or event publish
// the way the old loop did: log it, hold every cursor by ending the pass, and
// retry next tick. It is not a mailbox error, so nothing is relayed to the
// consumer and no error record is written for a control-plane hiccup.
func (w *WMail) controlPlaneError(err error, stats *tickStats) *errx.MailError {
	log.Warn().Err(err).Str("email_id", w.ID.String()).Msg("sync: control-plane call failed; pass ended, cursors held")
	stats.aborted = true
	return nil
}

func (w *WMail) mboxEvent(box *models.Mailbox) error {
	return w.onEvent(models.JobEventTypeMailboxUpdate, &models.JobEventMailboxUpdate{
		UserID:  w.UserID,
		EmailID: w.ID,
		Data:    box,
	})
}

func (w *SmtpImapData) FindPair(m *models.Mailbox) *models.Mailbox {
	for _, f := range w.Mailboxes {
		if f.UIDValidity == m.UIDValidity {
			return f
		}
	}
	return nil
}
