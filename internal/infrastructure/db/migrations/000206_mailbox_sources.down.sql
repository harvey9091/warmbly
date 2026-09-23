DROP TABLE IF EXISTS public.domain_redirects;
DELETE FROM public.mailbox_imports WHERE source IN ('vendor', 'workspace');
ALTER TABLE public.mailbox_imports DROP CONSTRAINT mailbox_imports_source_check;
ALTER TABLE public.mailbox_imports
    ADD CONSTRAINT mailbox_imports_source_check CHECK (source IN ('file', 'paste'));

DROP INDEX IF EXISTS public.idx_email_accounts_domain_grant;
DROP INDEX IF EXISTS public.idx_email_accounts_vendor_connection;

ALTER TABLE public.email_accounts
    DROP COLUMN IF EXISTS delegated_subject,
    DROP COLUMN IF EXISTS domain_grant_id,
    DROP COLUMN IF EXISTS vendor_mailbox_id,
    DROP COLUMN IF EXISTS vendor_connection_id;

DROP TABLE IF EXISTS public.mailbox_domain_grants;
DROP TABLE IF EXISTS public.mailbox_vendor_connections;
