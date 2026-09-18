package imap

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
)

// FindUIDByMessageID returns the UID of the message with the given RFC 5322
// Message-ID in mailboxName, or 0 when the folder does not have it.
//
// Warmup engagement arrives in two legs and the first one moves the message, so
// by the time the second runs the UID it carries addresses nothing in the folder
// the mail arrived in. This is the IMAP equivalent of re-resolving a Graph id:
// the Message-ID is the one identifier a move does not change.
func (c *Client) FindUIDByMessageID(ctx context.Context, mailboxName, rfcMessageID string) (uint32, error) {
	rfcMessageID = strings.TrimSpace(rfcMessageID)
	if mailboxName == "" || rfcMessageID == "" {
		return 0, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if merr := c.ensureConnected(); merr != nil {
		return 0, merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	name := c.qualifyMailboxLocked(mailboxName)
	if _, err := c.selectMailbox(name, nil); err != nil {
		// A folder that does not exist is not an error here: the caller is
		// asking whether the message is in it.
		return 0, nil
	}

	// SEARCH HEADER matches on a substring of the header value, so the angle
	// brackets are kept: a bare id would also match any message whose
	// References or In-Reply-To names it.
	data, err := c.client.UIDSearch(&imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "Message-Id", Value: "<" + strings.Trim(rfcMessageID, "<>") + ">"}},
	}, nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("search %q for message id: %w", name, err)
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return 0, nil
	}
	// Newest wins: a duplicate under an older UID is the copy a failed move
	// left behind.
	return uint32(uids[len(uids)-1]), nil
}

// MarkAsRead sets the \Seen flag on the given UID in mailboxName.
func (c *Client) MarkAsRead(ctx context.Context, mailboxName string, uid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if merr := c.ensureConnected(); merr != nil {
		return merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	if _, err := c.selectMailbox(mailboxName, nil); err != nil {
		return fmt.Errorf("select %q: %w", mailboxName, err)
	}

	cmd := c.client.Store(imap.UIDSetNum(imap.UID(uid)), &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}, nil)
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("store \\Seen: %w", err)
	}
	return nil
}

// MarkImportant sets the \Flagged flag on the given UID in mailboxName.
// Many IMAP UIs surface \Flagged as a star or important indicator.
func (c *Client) MarkImportant(ctx context.Context, mailboxName string, uid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if merr := c.ensureConnected(); merr != nil {
		return merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	if _, err := c.selectMailbox(mailboxName, nil); err != nil {
		return fmt.Errorf("select %q: %w", mailboxName, err)
	}

	cmd := c.client.Store(imap.UIDSetNum(imap.UID(uid)), &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagFlagged},
	}, nil)
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("store \\Flagged: %w", err)
	}
	return nil
}

// RemoveFromSpam moves the UID from sourceMailbox into inboxName.
// inboxName is usually "INBOX" but is provided so callers can supply
// the value resolved from the worker's mailbox list.
func (c *Client) RemoveFromSpam(ctx context.Context, sourceMailbox, inboxName string, uid uint32) error {
	if !IsSpamMailboxName(sourceMailbox) {
		return nil
	}
	return c.moveUID(ctx, sourceMailbox, inboxName, uid)
}

// MoveToFolder moves the UID from sourceMailbox into dstFolder, creating
// dstFolder if it does not exist. Use for the warmup sorting folder.
//
// A message already in the destination is left alone. Warmup mail can arrive
// there directly — a server-side rule, or another tool's filter, put it in the
// folder we were going to move it to — and a MOVE onto the same mailbox is a
// copy and an expunge, so the message would come back under a new UID and be
// re-imported as a fresh arrival on the next pass.
// It reports whether the message actually moved, because only then is the UID
// void: the caller has more to do with it when it did not.
func (c *Client) MoveToFolder(ctx context.Context, sourceMailbox, dstFolder string, uid uint32) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if merr := c.ensureConnected(); merr != nil {
		return false, merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	// Qualified on both sides: the caller names a bare folder, the server names
	// the one it listed, and on a Dovecot that keeps user folders under "INBOX."
	// those two spellings of the same mailbox are not equal as strings.
	dst := c.qualifyMailboxLocked(dstFolder)
	if strings.EqualFold(c.qualifyMailboxLocked(sourceMailbox), dst) {
		return false, nil
	}
	if err := c.ensureMailboxExists(dst); err != nil {
		return false, err
	}

	if err := c.moveUIDLocked(sourceMailbox, dst, uid); err != nil {
		return false, err
	}
	return true, nil
}

// personalPrefixLocked is the prefix this server keeps user folders under. It
// is "" almost everywhere, but Dovecot is commonly configured with "INBOX.",
// where creating a folder at the root fails with "nonexistent namespace" and
// the Warmbly foldering silently never happens. Cached per connection; a
// server without NAMESPACE keeps the root. mu must be held.
func (c *Client) personalPrefixLocked() string {
	if c.nsPrefix != nil {
		return *c.nsPrefix
	}
	prefix := ""
	if c.client.Caps().Has(imap.CapNamespace) {
		if data, err := c.client.Namespace().Wait(); err == nil && data != nil && len(data.Personal) > 0 {
			prefix = data.Personal[0].Prefix
		}
	}
	c.nsPrefix = &prefix
	return prefix
}

// qualifyMailboxLocked puts a bare folder name inside the personal namespace,
// leaving a name that already carries the prefix untouched. mu must be held.
func (c *Client) qualifyMailboxLocked(name string) string {
	prefix := c.personalPrefixLocked()
	if prefix == "" || strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func (c *Client) moveUID(ctx context.Context, src, dst string, uid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if merr := c.ensureConnected(); merr != nil {
		return merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	return c.moveUIDLocked(src, dst, uid)
}

func (c *Client) moveUIDLocked(src, dst string, uid uint32) error {
	if _, err := c.selectMailbox(src, nil); err != nil {
		return fmt.Errorf("select %q: %w", src, err)
	}

	set := imap.UIDSetNum(imap.UID(uid))
	if c.client.Caps().Has(imap.CapMove) {
		if _, err := c.client.Move(set, dst).Wait(); err != nil {
			return fmt.Errorf("move uid %d %q→%q: %w", uid, src, dst, err)
		}
		return nil
	}

	// MOVE not supported — emulate via COPY + STORE \Deleted + EXPUNGE.
	if _, err := c.client.Copy(set, dst).Wait(); err != nil {
		return fmt.Errorf("copy uid %d %q→%q: %w", uid, src, dst, err)
	}
	storeCmd := c.client.Store(set, &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}, nil)
	if err := storeCmd.Close(); err != nil {
		return fmt.Errorf("store \\Deleted on uid %d: %w", uid, err)
	}
	if err := c.client.Expunge().Close(); err != nil {
		return fmt.Errorf("expunge %q: %w", src, err)
	}
	return nil
}

func (c *Client) ensureMailboxExists(name string) error {
	list := c.client.List("", name, nil)
	found := false
	for f := list.Next(); f != nil; f = list.Next() {
		if f.Mailbox == name {
			found = true
		}
	}
	if err := list.Close(); err != nil {
		return fmt.Errorf("list mailbox %q: %w", name, err)
	}
	if found {
		return nil
	}
	if err := c.client.Create(name, nil).Wait(); err != nil {
		return fmt.Errorf("create mailbox %q: %w", name, err)
	}
	return nil
}

// IsSpamMailboxName returns true if the mailbox name is a Junk/Spam folder.
// Used as a guard so we never accidentally MOVE a non-spam message, so it
// matches the leaf exactly rather than by substring: a user folder called
// "Spam reports" holds mail its owner wants kept where it is.
func IsSpamMailboxName(name string) bool {
	return matchesFolderName(strings.ToLower(leaf(strings.TrimSpace(name))), ImapSpam)
}

// IsSpamMailbox returns true if the mailbox's attributes or name identify it
// as a Junk/Spam folder under RFC 6154 SPECIAL-USE or by name match.
func IsSpamMailbox(name string, attrs []string) bool {
	for _, a := range attrs {
		if strings.EqualFold(a, string(imap.MailboxAttrJunk)) {
			return true
		}
	}
	return IsSpamMailboxName(name)
}

// IsArchiveMailbox returns true if the mailbox's attributes or name identify it
// as the archive, under RFC 6154 SPECIAL-USE or by name match. \All is
// included because Gmail-over-IMAP and a few hosted servers expose their
// archive as the "all mail" view and nothing else.
func IsArchiveMailbox(name string, attrs []string) bool {
	for _, a := range attrs {
		switch strings.ToLower(a) {
		case "\\archive", "\\all":
			return true
		}
	}
	return matchesFolderName(strings.ToLower(leaf(strings.TrimSpace(name))), ImapArchive)
}

// IsSentMailbox returns true if the mailbox's attributes or name identify it as
// the Sent folder, under RFC 6154 SPECIAL-USE or by name match.
func IsSentMailbox(name string, attrs []string) bool {
	for _, a := range attrs {
		if strings.EqualFold(a, "\\Sent") {
			return true
		}
	}
	return matchesFolderName(strings.ToLower(leaf(strings.TrimSpace(name))), ImapSent)
}

// IsInboxMailbox returns true for the canonical INBOX (case-insensitive) or
// any mailbox flagged with the \Inbox special-use attribute.
func IsInboxMailbox(name string, attrs []string) bool {
	if strings.EqualFold(strings.TrimSpace(name), "INBOX") {
		return true
	}
	for _, a := range attrs {
		if strings.EqualFold(a, "\\Inbox") {
			return true
		}
	}
	return false
}

// SetSeen adds or removes \Seen across many UIDs in one STORE.
//
// One command for the whole set, because the alternative is a SELECT and a
// STORE per message and the unibox files them in bulk. UIDs the server no
// longer has are silently ignored by STORE, which is what should happen: the
// message moved or was deleted at the provider and the next sync will say so.
func (c *Client) SetSeen(ctx context.Context, mailboxName string, uids []uint32, seen bool) error {
	if len(uids) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if merr := c.ensureConnected(); merr != nil {
		return merr
	}
	c.lifecycle.RLock()
	defer c.lifecycle.RUnlock()
	defer c.begin()()
	if _, err := c.selectMailbox(mailboxName, nil); err != nil {
		return fmt.Errorf("select %q: %w", mailboxName, err)
	}

	set := imap.UIDSet{}
	for _, uid := range uids {
		set.AddNum(imap.UID(uid))
	}
	op := imap.StoreFlagsDel
	if seen {
		op = imap.StoreFlagsAdd
	}

	cmd := c.client.Store(set, &imap.StoreFlags{
		Op:     op,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}, nil)
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("store \\Seen (%d uids): %w", len(uids), err)
	}
	return nil
}
