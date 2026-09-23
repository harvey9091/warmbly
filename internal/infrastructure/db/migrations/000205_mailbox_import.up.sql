-- Where a mailbox's mail is hosted and how it signs in, independent of the
-- transport: a Google Workspace mailbox on an app password is provider
-- smtp_imap, mail_host google_workspace, auth_method app_password. mail_host is
-- filled by the connect path and, for older rows, by the domain sweep.
ALTER TABLE public.email_accounts
    ADD COLUMN mail_host text NOT NULL DEFAULT '',
    ADD COLUMN auth_method text NOT NULL DEFAULT '';

ALTER TABLE public.email_accounts
    ADD CONSTRAINT email_accounts_mail_host_check CHECK (mail_host ~ '^[a-z0-9_]{0,32}$'),
    ADD CONSTRAINT email_accounts_auth_method_check
        CHECK (auth_method IN ('', 'password', 'app_password', 'oauth', 'delegated'));

UPDATE public.email_accounts SET auth_method = 'oauth' WHERE provider IN ('gmail', 'outlook');

-- The duplicate check at connect time is per workspace and case-insensitive.
CREATE INDEX idx_email_accounts_org_email ON public.email_accounts (organization_id, lower(email));

-- One import of many mailboxes, worked off in the background so it survives a
-- closed tab and a restarted backend.
CREATE TABLE public.mailbox_imports (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    created_by uuid REFERENCES public.users (id) ON DELETE SET NULL,
    source text NOT NULL CHECK (source IN ('file', 'paste')),
    filename text NOT NULL DEFAULT '',
    vendor text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'cancelled')),
    on_existing text NOT NULL DEFAULT 'update' CHECK (on_existing IN ('update', 'skip')),
    settings jsonb NOT NULL DEFAULT '{}'::jsonb,
    columns jsonb NOT NULL DEFAULT '[]'::jsonb,
    total integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    -- Sealed row credentials are dropped after this, so a retry stops being possible.
    credentials_expire_at timestamptz
);

CREATE INDEX idx_mailbox_imports_org ON public.mailbox_imports (organization_id, created_at DESC);

CREATE TABLE public.mailbox_import_rows (
    import_id uuid NOT NULL REFERENCES public.mailbox_imports (id) ON DELETE CASCADE,
    line integer NOT NULL,
    email text NOT NULL,
    domain text NOT NULL DEFAULT '',
    mail_host text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'queued' CHECK (status IN (
        'queued', 'running', 'connected', 'updated', 'skipped', 'failed', 'needs_signin', 'cancelled'
    )),
    -- Credentials and per-row settings sealed with the organization's DEK; '' once spent.
    payload text NOT NULL DEFAULT '',
    -- The uploaded row's non-secret cells, for the failed-rows download.
    fields jsonb NOT NULL DEFAULT '{}'::jsonb,
    code text NOT NULL DEFAULT '',
    cause text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    email_account_id uuid REFERENCES public.email_accounts (id) ON DELETE SET NULL,
    attempts integer NOT NULL DEFAULT 0,
    lease_until timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (import_id, line)
);

CREATE INDEX idx_mailbox_import_rows_due ON public.mailbox_import_rows (lease_until NULLS FIRST)
    WHERE status IN ('queued', 'running');
CREATE INDEX idx_mailbox_import_rows_status ON public.mailbox_import_rows (import_id, status);
CREATE INDEX idx_mailbox_import_rows_signin ON public.mailbox_import_rows (lower(email))
    WHERE status = 'needs_signin';

-- A confirmed column mapping, keyed by the file's header set, so the next file
-- with the same headers maps itself.
CREATE TABLE public.mailbox_import_mappings (
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    signature text NOT NULL,
    mapping jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, signature)
);
