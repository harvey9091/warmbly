-- A mailbox owner can name folders the sync leaves alone.
--
-- The IMAP sync follows every folder the server lists, and a folder it does
-- not recognise files as inbox so the mail in it stays visible. A folder a
-- third-party tool fills with its own machine traffic then lands in the
-- unified inbox, spends the mailbox's sync budget, and is classified like a
-- reply. The names here are matched against the server's listing and the
-- folders they name (and their subfolders) are never opened.
--
-- email_accounts.sync_skip_folders: folder names as the server lists them.
--   Empty means everything is synced. The special folders (inbox, sent,
--   drafts, spam, trash, archive) cannot be named here.
ALTER TABLE public.email_accounts
    ADD COLUMN sync_skip_folders text[] NOT NULL DEFAULT '{}';
