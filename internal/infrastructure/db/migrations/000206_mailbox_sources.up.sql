-- An inbox vendor account (InboxKit, Zapmail, ...) the workspace imports from.
-- The API key and any account ids are sealed with the organization's DEK.
CREATE TABLE public.mailbox_vendor_connections (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    vendor text NOT NULL CHECK (vendor IN (
        'inboxkit', 'zapmail', 'mailforge', 'infraforge', 'maildoso', 'cheapinboxes', 'scaledmail'
    )),
    label text NOT NULL DEFAULT '',
    credentials text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'invalid')),
    last_error text NOT NULL DEFAULT '',
    last_used_at timestamptz,
    created_by uuid REFERENCES public.users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_mailbox_vendor_connections_org ON public.mailbox_vendor_connections (organization_id);

-- An administrator's grant over a whole Google Workspace domain (domain-wide
-- delegation) or Microsoft 365 tenant (application consent). Tokens are minted
-- on demand from the instance's own credentials; none is stored here.
CREATE TABLE public.mailbox_domain_grants (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('google', 'microsoft')),
    -- Google: the primary domain. Microsoft: the tenant id.
    tenant text NOT NULL,
    -- Google: the administrator impersonated to read the directory.
    admin_email text NOT NULL DEFAULT '',
    -- Every domain the grant covers, lower-cased.
    domains text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'invalid')),
    last_error text NOT NULL DEFAULT '',
    verified_at timestamptz,
    created_by uuid REFERENCES public.users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, provider, tenant)
);

ALTER TABLE public.email_accounts
    ADD COLUMN vendor_connection_id uuid REFERENCES public.mailbox_vendor_connections (id) ON DELETE SET NULL,
    ADD COLUMN vendor_mailbox_id text NOT NULL DEFAULT '',
    ADD COLUMN domain_grant_id uuid REFERENCES public.mailbox_domain_grants (id) ON DELETE SET NULL,
    -- The identity a delegated token is minted for: the address (Google) or the Graph user id (Microsoft).
    ADD COLUMN delegated_subject text NOT NULL DEFAULT '';

CREATE INDEX idx_email_accounts_vendor_connection ON public.email_accounts (vendor_connection_id)
    WHERE vendor_connection_id IS NOT NULL;
CREATE INDEX idx_email_accounts_domain_grant ON public.email_accounts (domain_grant_id)
    WHERE domain_grant_id IS NOT NULL;

ALTER TABLE public.mailbox_imports DROP CONSTRAINT mailbox_imports_source_check;
ALTER TABLE public.mailbox_imports
    ADD CONSTRAINT mailbox_imports_source_check CHECK (source IN ('file', 'paste', 'vendor', 'workspace'));

-- A sending domain's root (and www) answered by this instance with a redirect
-- to the workspace's main website. Served only once verified: a TXT record
-- proves the workspace controls the domain, an address record that it points here.
CREATE TABLE public.domain_redirects (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    domain text NOT NULL CHECK (domain = lower(domain) AND domain <> ''),
    target_url text NOT NULL,
    include_www boolean NOT NULL DEFAULT true,
    verify_token text NOT NULL,
    verified boolean NOT NULL DEFAULT false,
    verified_at timestamptz,
    last_checked_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_by uuid REFERENCES public.users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, domain)
);

-- One redirect answers for a hostname across the instance.
CREATE UNIQUE INDEX idx_domain_redirects_verified_domain ON public.domain_redirects (domain) WHERE verified;
CREATE INDEX idx_domain_redirects_check ON public.domain_redirects (last_checked_at NULLS FIRST);
