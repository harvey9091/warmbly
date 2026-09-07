-- UIDNEXT per folder: the incremental cursor on IMAP servers without
-- CONDSTORE, where highestmodseq stays 0.
ALTER TABLE unibox_mailboxes ADD COLUMN IF NOT EXISTS uid_next bigint NOT NULL DEFAULT 0;
