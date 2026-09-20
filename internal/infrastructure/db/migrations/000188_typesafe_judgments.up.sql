-- Typed judgments across the product, all read from the shared TypeSafe client.
--
-- Inbox tagging phases 2 and 3: a classification may now hold a lead, stop a
-- declined sequence, open a task or suppress an address, each behind its own
-- workspace switch. What was done is recorded next to the verdict so the review
-- page shows the action with the answer that caused it.
ALTER TABLE public.inbox_tag_results
    ADD COLUMN IF NOT EXISTS actions text[] NOT NULL DEFAULT '{}';

-- A hold written from a classified reply is its own source, so a member's own
-- pause and an out-of-office hold are never mistaken for it.
ALTER TABLE public.campaign_leads DROP CONSTRAINT IF EXISTS campaign_leads_pause_source_check;
ALTER TABLE public.campaign_leads
    ADD CONSTRAINT campaign_leads_pause_source_check
    CHECK (pause_source IS NULL OR pause_source IN ('manual', 'out_of_office', 'inbox_tagging'));

-- The classified intent of the contact's reply (agreed, wants_pricing,
-- not_now, ...), next to the coarse reply_class the branch conditions already
-- read. Empty when no classification reached the row.
ALTER TABLE public.campaign_contact_progress
    ADD COLUMN IF NOT EXISTS reply_intent text NOT NULL DEFAULT '';

-- A judgment of one step's copy, keyed by the hash of the copy it judged, so
-- the Advisor re-reads a step only when its words change. A cache: it is
-- recomputed on the destination rather than exported.
CREATE TABLE IF NOT EXISTS public.copy_judgments (
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    content_hash    text NOT NULL,
    verdict         jsonb NOT NULL DEFAULT '{}'::jsonb,
    model           text NOT NULL DEFAULT '',
    input_tokens    integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, content_hash),
    CONSTRAINT copy_judgments_verdict_check CHECK (jsonb_typeof(verdict) = 'object')
);

-- Form triage: a per-form switch, and the verdict on each submission.
ALTER TABLE public.forms
    ADD COLUMN IF NOT EXISTS triage_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE public.form_submissions
    ADD COLUMN IF NOT EXISTS triage text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS triage_confidence real NOT NULL DEFAULT 0;

ALTER TABLE public.form_submissions DROP CONSTRAINT IF EXISTS form_submissions_triage_check;
ALTER TABLE public.form_submissions
    ADD CONSTRAINT form_submissions_triage_check
    CHECK (triage IN ('', 'buyer', 'vendor', 'job_seeker', 'other', 'junk'));
