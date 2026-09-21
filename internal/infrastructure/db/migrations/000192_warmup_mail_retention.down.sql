ALTER TABLE public.warmup_tokens
    DROP COLUMN IF EXISTS sent_retired_at;

ALTER TABLE public.warmup_received
    DROP COLUMN IF EXISTS retired_at;

ALTER TABLE public.email_accounts
    DROP CONSTRAINT IF EXISTS email_accounts_warmup_retention_days_check,
    DROP COLUMN IF EXISTS warmup_retention_days;
