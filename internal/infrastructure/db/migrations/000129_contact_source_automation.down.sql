-- Rows stamped 'automation' fall back to 'unknown' so the narrower CHECK holds.
UPDATE public.contacts SET source = 'unknown' WHERE source = 'automation';
-- NOT VALID skips the table scan under the migration's lock; the rows already
-- satisfy the set, so nothing needs validating.
ALTER TABLE public.contacts DROP CONSTRAINT contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant', 'form')) NOT VALID;
