-- A contact created by an automation's "create or update contact" action is a
-- first-touch origin of its own, so an inbound lead (a Zapier, Make or n8n
-- push, a lead-ads form) reads as what it is instead of "unknown".
-- NOT VALID skips the table scan under the migration's lock; the rows already
-- satisfy the set, so nothing needs validating.
ALTER TABLE public.contacts DROP CONSTRAINT contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant', 'form', 'automation')) NOT VALID;
