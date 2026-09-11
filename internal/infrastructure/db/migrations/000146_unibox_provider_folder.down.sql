-- Back to one folder column. Any message the user filed in Warmbly rather
-- than at the provider keeps that placement until the next provider move
-- overwrites it, which is the pre-split behaviour.
ALTER TABLE public.unibox_emails
    DROP CONSTRAINT IF EXISTS unibox_emails_provider_folder_check;

ALTER TABLE public.unibox_emails
    DROP COLUMN IF EXISTS provider_folder;
