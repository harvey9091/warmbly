-- Where the PROVIDER has the message, tracked apart from where Warmbly shows
-- it, so the two can disagree.
--
-- Until now unibox_emails.folder was both at once: the sync wrote the
-- provider's placement into it and every read treated it as the placement.
-- That is fine while the provider is the only thing that files mail, and it
-- stops being fine the moment the thread header can Archive or Delete a
-- conversation. Those actions move the message here and not at the provider,
-- so the next flag scan of the provider's INBOX reports inbox again and pulls
-- it straight back out.
--
-- Suppressing "provider says inbox" outright would be worse: it is also what
-- an ordinary un-archive at the provider looks like, and refusing it would
-- strand the message here forever. With both values stored, the rule is
-- exact. The store follows the provider only when the PROVIDER's folder
-- actually changed from the one last observed; a scan that keeps reporting
-- the same folder changes nothing, and a local filing survives it.
--
-- Cost note: unibox_emails holds every synced message, so the backfill below
-- is the expensive part of this migration. The backend applies migrations at
-- boot inside one transaction and blocks until they finish, so deploy this in
-- a window rather than alongside traffic. ADD COLUMN with a constant DEFAULT
-- is metadata-only on PG 11+ and is not itself a rewrite.
ALTER TABLE public.unibox_emails
    ADD COLUMN provider_folder text NOT NULL DEFAULT '';

ALTER TABLE public.unibox_emails
    ADD CONSTRAINT unibox_emails_provider_folder_check
    CHECK (provider_folder IN ('', 'inbox', 'sent', 'drafts', 'archive', 'spam', 'trash'));

-- Every existing row was filed by the provider and by nothing else, so the
-- two values start out equal and no historical message reads as locally
-- filed. '' stays reachable for a row written by a consumer that predates
-- this column; the handler treats it as "never observed" and adopts.
UPDATE public.unibox_emails SET provider_folder = folder;
