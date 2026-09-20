ALTER TABLE public.form_submissions DROP CONSTRAINT IF EXISTS form_submissions_triage_check;
ALTER TABLE public.form_submissions
    DROP COLUMN IF EXISTS triage,
    DROP COLUMN IF EXISTS triage_confidence;
ALTER TABLE public.forms DROP COLUMN IF EXISTS triage_enabled;

DROP TABLE IF EXISTS public.copy_judgments;

ALTER TABLE public.campaign_contact_progress DROP COLUMN IF EXISTS reply_intent;

UPDATE public.campaign_leads
SET pause_source = 'manual'
WHERE pause_source = 'inbox_tagging';
ALTER TABLE public.campaign_leads DROP CONSTRAINT IF EXISTS campaign_leads_pause_source_check;
ALTER TABLE public.campaign_leads
    ADD CONSTRAINT campaign_leads_pause_source_check
    CHECK (pause_source IS NULL OR pause_source IN ('manual', 'out_of_office'));

ALTER TABLE public.inbox_tag_results DROP COLUMN IF EXISTS actions;
