-- A continuous campaign stays active when it runs out of leads and waits for
-- more instead of finishing (issue #336). idle_since marks that wait.
ALTER TABLE public.campaigns
    ADD COLUMN continuous BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN idle_since TIMESTAMPTZ;

-- A campaign fed by a linked segment is exactly the case this exists for.
UPDATE public.campaigns SET continuous = true
WHERE id IN (SELECT campaign_id FROM public.campaign_segments);
