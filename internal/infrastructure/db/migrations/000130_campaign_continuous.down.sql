ALTER TABLE public.campaigns
    DROP COLUMN IF EXISTS idle_since,
    DROP COLUMN IF EXISTS continuous;
