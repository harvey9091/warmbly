-- A mail folder is identified by its name, not by its UIDVALIDITY.
--
-- UIDVALIDITY was the folder's identity everywhere: the primary key of this
-- table, the number DELETE_MAILBOX carried, and the stamp on every stored
-- message. RFC 3501 does not support that. It promises only that UIDs are
-- stable WITHIN one folder while its UIDVALIDITY is unchanged, and says
-- nothing about the value being unique ACROSS folders. Servers that derive it
-- from the folder's creation time, Dovecot among them, hand the same number
-- to every folder created in the same second, which is what a folder tree
-- made by a mail client, an import or a server migration is by definition.
--
-- The sync loop contained that by following one folder per id and reporting
-- the rest, so the others' mail was never synced. Keying by name fixes it at
-- the root: IMAP does guarantee a name is unique per account.
--
-- UIDVALIDITY keeps the job it actually has. It is the validity marker for
-- the UIDs we hold, so it stays on the folder row (the cursor is void when it
-- changes) and on each message (its stored uid belongs to that generation).
--
-- Cost note: the backfill below walks unibox_emails, which holds every synced
-- message, so on a long-running instance it is the expensive part. The
-- backend applies migrations at boot inside one transaction and blocks until
-- they finish, so deploy this in a window rather than alongside traffic.
-- ADD COLUMN with a constant DEFAULT is metadata-only on PG 11+ and is not
-- itself a rewrite.

-- A row with no name cannot be addressed by one. It predates the name being
-- an identity; the next sync pass re-creates it from the listing.
DELETE FROM public.unibox_mailboxes WHERE mailbox = '';

-- Collapse names that appear more than once. Under the old primary key a
-- folder whose UIDVALIDITY changed inserted a second row and left the first
-- behind, so the same name can be here twice. Keep the one the sync touched
-- last, breaking a tie on the higher UIDVALIDITY, which is the newer
-- generation on every server that derives it from a clock or a counter.
DELETE FROM public.unibox_mailboxes um
USING public.unibox_mailboxes other
WHERE um.email_id = other.email_id
  AND um.mailbox = other.mailbox
  AND (um.updated_at, um.uid_validity) < (other.updated_at, other.uid_validity);

ALTER TABLE public.unibox_mailboxes DROP CONSTRAINT unibox_mailboxes_pkey;
ALTER TABLE public.unibox_mailboxes
    ADD CONSTRAINT unibox_mailboxes_pkey PRIMARY KEY (email_id, mailbox);

-- folder_path is the message's source folder by name: stable across a
-- UIDVALIDITY change, and the thing a rename moves rather than orphans.
-- unibox_emails.mailbox keeps its own meaning as the UIDVALIDITY generation
-- the stored uid belongs to.
ALTER TABLE public.unibox_emails
    ADD COLUMN folder_path text NOT NULL DEFAULT '';

UPDATE public.unibox_emails ue
SET folder_path = um.mailbox
FROM public.unibox_mailboxes um
WHERE um.email_id = ue.email_id
  AND um.uid_validity = ue.mailbox
  AND ue.mailbox <> 0;

CREATE INDEX idx_unibox_emails_folder_path ON public.unibox_emails (email_id, folder_path);

-- The backfill cursor keys its per-folder floors by folder. On IMAP those
-- keys were UIDVALIDITY rendered as a string; rewrite them to names so a
-- backfill in flight resumes instead of re-walking every folder from the top.
-- Graph accounts already key by folder name and fall through the LEFT JOIN
-- unchanged.
UPDATE public.email_sync_state ess
SET backfill_cursor = jsonb_set(
        ess.backfill_cursor,
        '{folders}',
        (
            SELECT COALESCE(jsonb_object_agg(COALESCE(um.mailbox, f.key), f.value), '{}'::jsonb)
            FROM jsonb_each(ess.backfill_cursor -> 'folders') AS f(key, value)
            LEFT JOIN public.unibox_mailboxes um
                   ON um.email_id = ess.email_id
                  AND um.uid_validity::text = f.key
        )
    )
WHERE jsonb_typeof(ess.backfill_cursor -> 'folders') = 'object'
  AND ess.backfill_cursor -> 'folders' <> '{}'::jsonb;
