DROP TABLE IF EXISTS public.mailbox_import_mappings;
DROP TABLE IF EXISTS public.mailbox_import_rows;
DROP TABLE IF EXISTS public.mailbox_imports;

DROP INDEX IF EXISTS public.idx_email_accounts_org_email;

ALTER TABLE public.email_accounts
    DROP CONSTRAINT IF EXISTS email_accounts_auth_method_check,
    DROP CONSTRAINT IF EXISTS email_accounts_mail_host_check,
    DROP COLUMN IF EXISTS auth_method,
    DROP COLUMN IF EXISTS mail_host;
