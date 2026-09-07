package imap

import (
	"errors"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// Folders lists the selectable folders of the account with the cursors the
// sync loop keys on (UIDVALIDITY, UIDNEXT and, on a CONDSTORE server,
// HIGHESTMODSEQ).
//
// "*", not "%": "%" stops at the top level, and on Gmail-over-IMAP every
// folder but INBOX lives under "[Gmail]/" (Dovecot commonly under "INBOX."),
// so Sent, Spam and Trash were never listed and never synced.
//
// The listing is capped at config.MaxEmailFolders, INBOX and the special
// folders first so a mailbox with hundreds of user folders still syncs what
// matters; FolderOverflow reports how many were left out.
func (c *Client) Folders() ([]models.Mailbox, *errx.MailError) {
	return c.foldersCapped(config.MaxEmailFolders)
}

// foldersCapped is Folders with the cap injectable, so a test can exercise
// the overflow without standing up a hundred folders.
func (c *Client) foldersCapped(limit int) ([]models.Mailbox, *errx.MailError) {
	if err := c.ensureConnected(); err != nil {
		return nil, err
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()

	caps := c.client.Caps()
	status := &imap.StatusOptions{
		UIDValidity: true,
		UIDNext:     true,
		// Asking a server without CONDSTORE for HIGHESTMODSEQ is a BAD.
		HighestModSeq: caps.Has(imap.CapCondStore),
	}
	opts := &imap.ListOptions{}
	// LIST-STATUS folds the STATUS of every folder into the one round trip;
	// without it each kept folder is asked separately below.
	listStatus := caps.Has(imap.CapListStatus)
	if listStatus {
		opts.ReturnStatus = status
	}
	// Gmail attaches \Sent, \Trash, \Junk, \All ... only when asked; on a
	// plain LIST every folder is just \HasNoChildren and the canonical-folder
	// mapping is left guessing from names ("Bin" filed as inbox).
	if caps.Has(imap.CapSpecialUse) {
		opts.ReturnSpecialUse = true
	}

	var all []models.Mailbox
	statuses := map[string]*imap.StatusData{}
	cmd := c.client.List("", "*", opts)
	for f := cmd.Next(); f != nil; f = cmd.Next() {
		attrs := make([]string, len(f.Attrs))
		for i := range f.Attrs {
			attrs[i] = string(f.Attrs[i])
		}
		box := models.Mailbox{Name: f.Mailbox, Attrs: attrs, Delim: delimString(f.Delim)}
		if !selectableFolder(attrs) || IsVirtualFolder(box) {
			continue
		}
		all = append(all, box)
		if f.Status != nil {
			statuses[f.Mailbox] = f.Status
		}
	}
	if err := cmd.Close(); err != nil {
		return nil, c.handleError(err)
	}

	kept, overflow := rankFolders(all, limit)
	c.folderOverflow.Store(int32(overflow))

	resp := make([]models.Mailbox, 0, len(kept))
	for _, box := range kept {
		st := statuses[box.Name]
		if st == nil {
			if listStatus {
				// The server was asked and said nothing: the folder is not
				// one it can open for us.
				continue
			}
			data, err := c.client.Status(box.Name, status).Wait()
			if err != nil {
				var imapErr *imap.Error
				if errors.As(err, &imapErr) {
					// One folder the server will not report on must not
					// take the rest of the account with it.
					log.Warn().Err(err).Str("folder", box.Name).Msg("imap: STATUS refused; folder skipped")
					continue
				}
				return nil, c.handleError(err)
			}
			st = data
		}
		box.UIDValidity = st.UIDValidity
		box.UIDNext = uint32(st.UIDNext)
		box.HighestModSeq = st.HighestModSeq
		resp = append(resp, box)
	}

	resp, conflicts := dedupeByUIDValidity(resp)
	c.folderConflicts.Store(int32(conflicts))
	return resp, nil
}

// dedupeByUIDValidity keeps one folder per UIDVALIDITY.
//
// Everything downstream identifies a folder by that number, including the
// primary key of the stored folder row, but RFC 3501 only promises UIDs are
// stable within one folder: Dovecot and others derive UIDVALIDITY from the
// creation time, so a folder tree created in the same second shares one.
// Two folders under a single id would advance each other's cursor and delete
// each other's row, which loses mail. Dropping the later one leaves it
// unsynced (and says so) but leaves every other folder correct. Input is
// already ranked, so the inbox and the special folders win any collision.
func dedupeByUIDValidity(boxes []models.Mailbox) ([]models.Mailbox, int) {
	seen := make(map[uint32]string, len(boxes))
	kept := boxes[:0]
	conflicts := 0
	for _, box := range boxes {
		if other, dup := seen[box.UIDValidity]; dup {
			log.Warn().
				Str("folder", box.Name).
				Str("conflicts_with", other).
				Uint32("uid_validity", box.UIDValidity).
				Msg("imap: two folders report the same UIDVALIDITY; the second is not synced")
			conflicts++
			continue
		}
		seen[box.UIDValidity] = box.Name
		kept = append(kept, box)
	}
	return kept, conflicts
}

// FolderOverflow is how many selectable folders the last Folders call left
// out because the account has more than config.MaxEmailFolders.
func (c *Client) FolderOverflow() int {
	return int(c.folderOverflow.Load())
}

// FolderConflicts is how many folders the last Folders call left out because
// another folder reported the same UIDVALIDITY.
func (c *Client) FolderConflicts() int {
	return int(c.folderConflicts.Load())
}

// rankFolders orders a listing INBOX first, then the special folders (sent,
// drafts, junk, trash, archive), then the rest in the server's order, and
// cuts it at limit. Ties keep the server's order, so the result is stable
// from one pass to the next.
//
// The ranking applies whether or not anything is cut, because the sync pass
// walks folders in this order and can run out of budget partway: a reply to
// the customer's own outreach should land before a mailing list in a user
// folder does, whatever order the server happened to list them in.
func rankFolders(all []models.Mailbox, limit int) ([]models.Mailbox, int) {
	rank := func(box *models.Mailbox) int {
		switch {
		case strings.EqualFold(box.Name, "INBOX"):
			return 0
		case CanonicalFolder(*box) != models.FolderInbox:
			return 1
		}
		return 2
	}
	sort.SliceStable(all, func(i, j int) bool { return rank(&all[i]) < rank(&all[j]) })
	if len(all) <= limit {
		return all, 0
	}
	return all[:limit], len(all) - limit
}

// selectableFolder is false for the containers a server lists only to show
// hierarchy (\Noselect) and the placeholders of LIST-EXTENDED (\NonExistent).
func selectableFolder(attrs []string) bool {
	for _, a := range attrs {
		switch strings.ToLower(a) {
		case "\\noselect", "\\nonexistent":
			return false
		}
	}
	return true
}

// IsVirtualFolder is a Gmail label view (All Mail, Starred, Important):
// every message in it also lives in a real folder under a different UID, so
// syncing it would re-file known mail (All Mail reads as archive) and swap
// the (mailbox, uid) pair the warmup actions address. A message archived out
// of every real folder stays unsynced, which is the ceiling of
// Gmail-over-IMAP; the OAuth Gmail path has no such gap.
func IsVirtualFolder(box models.Mailbox) bool {
	for _, a := range box.Attrs {
		switch strings.ToLower(a) {
		case "\\all", "\\flagged", "\\important":
			return true
		}
	}
	// Name fallback only inside Gmail's own namespace: a plain IMAP server
	// can legitimately have a user folder called "Important" or "Starred".
	// Gmail's delimiter is always "/", so this does not need the server's.
	lower := strings.ToLower(box.Name)
	if !strings.HasPrefix(lower, "[gmail]/") && !strings.HasPrefix(lower, "[google mail]/") {
		return false
	}
	switch lower[strings.Index(lower, "/")+1:] {
	case "all mail", "starred", "important":
		return true
	}
	return false
}

// BackfillEligible excludes folders whose history is not worth importing:
// trash, spam and Gmail's virtual views. Live sync still follows trash and
// spam for placement signals and to file new mail into those scopes; only the
// bounded initial import skips them, because their history would consume the
// message budget that belongs to real conversations. Drafts IS imported: it is
// small and a Drafts scope with none of the mailbox's existing drafts in it
// reads as broken.
func BackfillEligible(box models.Mailbox) bool {
	if IsVirtualFolder(box) || !selectableFolder(box.Attrs) {
		return false
	}
	switch CanonicalFolder(box) {
	case models.FolderTrash, models.FolderSpam:
		return false
	}
	return true
}

// CanonicalFolder maps an IMAP folder to the canonical unibox folder.
// Special-use attributes are authoritative, with a name fallback for servers
// that do not advertise them; unrecognized user folders file as inbox so
// their mail stays visible.
func CanonicalFolder(box models.Mailbox) string {
	for _, a := range box.Attrs {
		switch strings.ToLower(a) {
		case "\\sent":
			return models.FolderSent
		case "\\drafts":
			return models.FolderDrafts
		case "\\junk":
			return models.FolderSpam
		case "\\trash":
			return models.FolderTrash
		case "\\archive", "\\all":
			return models.FolderArchive
		}
	}
	// The server reports its own hierarchy delimiter per folder, so a folder
	// whose name contains a dot on a "/" server is not cut in the middle.
	leafName := strings.ToLower(leafWithDelim(box.Name, box.Delim))
	switch {
	case matchesFolderName(leafName, ImapSent):
		return models.FolderSent
	case matchesFolderName(leafName, ImapDrafts):
		return models.FolderDrafts
	case matchesFolderName(leafName, ImapSpam):
		return models.FolderSpam
	case matchesFolderName(leafName, ImapTrash):
		return models.FolderTrash
	case matchesFolderName(leafName, ImapArchive):
		return models.FolderArchive
	}
	return models.FolderInbox
}

// delimString renders the delimiter LIST reported. go-imap carries it as a
// rune and a server that has no hierarchy reports NIL, which arrives as 0;
// converting that directly would produce a NUL byte and make every name look
// like it has no separator.
func delimString(delim rune) string {
	if delim == 0 {
		return ""
	}
	return string(delim)
}

// matchesFolderName compares an already-lowercased leaf against one of the
// role lists. Exact match only: "spam reports" is a user folder, not spam.
func matchesFolderName(leafName string, names []string) bool {
	for _, n := range names {
		if leafName == n {
			return true
		}
	}
	return false
}
