-- The enum value and any 'form'-sourced rows stay; only the tables go.
ALTER TABLE public.contacts DROP CONSTRAINT contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant', 'form'));

DROP TABLE IF EXISTS public.form_submissions;
DROP TABLE IF EXISTS public.form_categories;
DROP TABLE IF EXISTS public.forms;
