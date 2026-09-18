ALTER TABLE public.email_accounts
    DROP CONSTRAINT IF EXISTS email_accounts_warmup_placement_check,
    DROP COLUMN IF EXISTS warmup_placement,
    DROP COLUMN IF EXISTS warmup_folder;
