-- Validates the two CHECK constraints 000188 added NOT VALID. A VALIDATE only
-- takes a SHARE UPDATE EXCLUSIVE lock, so writes continue while it scans.
ALTER TABLE public.campaign_leads VALIDATE CONSTRAINT campaign_leads_pause_source_check;
ALTER TABLE public.form_submissions VALIDATE CONSTRAINT form_submissions_triage_check;
