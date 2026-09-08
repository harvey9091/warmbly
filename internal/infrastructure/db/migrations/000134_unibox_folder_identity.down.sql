-- Back to UIDVALIDITY as the folder's identity.
--
-- The backfill cursor keys are not rewritten back: a folder name that no
-- longer maps to a row would be lost, and the only cost of leaving them is
-- that an unfinished IMAP backfill re-walks its folders once.

DROP INDEX IF EXISTS idx_unibox_emails_folder_path;

ALTER TABLE public.unibox_emails DROP COLUMN IF EXISTS folder_path;

-- Two folders that shared a UIDVALIDITY could both be stored under the new
-- key but not under the old one, so drop the later ones first.
DELETE FROM public.unibox_mailboxes um
USING public.unibox_mailboxes other
WHERE um.email_id = other.email_id
  AND um.uid_validity = other.uid_validity
  AND (um.updated_at, um.mailbox) < (other.updated_at, other.mailbox);

ALTER TABLE public.unibox_mailboxes DROP CONSTRAINT unibox_mailboxes_pkey;
ALTER TABLE public.unibox_mailboxes
    ADD CONSTRAINT unibox_mailboxes_pkey PRIMARY KEY (email_id, uid_validity);
