-- Recreates the push-based provisioning tables and the SSH columns. Their
-- contents are gone: the servers, keys and templates they described were not
-- carried forward, so this restores the shape and nothing else.

BEGIN;

CREATE TYPE public.worker_install_state AS ENUM (
    'pending',
    'provisioning',
    'installed',
    'error',
    'uninstalling',
    'uninstalled'
);

ALTER TABLE workers
    ADD COLUMN IF NOT EXISTS ssh_host text,
    ADD COLUMN IF NOT EXISTS ssh_port integer DEFAULT 22 NOT NULL,
    ADD COLUMN IF NOT EXISTS ssh_user character varying(64) DEFAULT 'root'::character varying NOT NULL,
    ADD COLUMN IF NOT EXISTS ssh_public_key text,
    ADD COLUMN IF NOT EXISTS ssh_private_key_encrypted text,
    ADD COLUMN IF NOT EXISTS ssh_host_fingerprint text,
    ADD COLUMN IF NOT EXISTS install_state public.worker_install_state DEFAULT 'pending'::public.worker_install_state NOT NULL,
    ADD COLUMN IF NOT EXISTS enrollment_token_hash text,
    ADD COLUMN IF NOT EXISTS enrollment_token_expires_at timestamp with time zone,
    ADD COLUMN IF NOT EXISTS profile_id uuid,
    ADD COLUMN IF NOT EXISTS config_applied_at timestamp with time zone;

CREATE TABLE public.aws_credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(120) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    region character varying(40) NOT NULL,
    access_key_id text NOT NULL,
    secret_access_key_encrypted text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.worker_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(120) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    app_env character varying(20) DEFAULT 'prod'::character varying NOT NULL,
    worker_image text DEFAULT 'ghcr.io/warmbly/worker:latest'::text NOT NULL,
    kafka_bootstrap_servers text DEFAULT ''::text NOT NULL,
    kafka_sasl_username text DEFAULT ''::text NOT NULL,
    kafka_sasl_password_encrypted text DEFAULT ''::text NOT NULL,
    schema_registry_url text DEFAULT ''::text NOT NULL,
    schema_registry_key text DEFAULT ''::text NOT NULL,
    schema_registry_secret_encrypted text DEFAULT ''::text NOT NULL,
    redis_url_encrypted text DEFAULT ''::text NOT NULL,
    aws_credential_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    release_channel public.release_channel DEFAULT 'pinned'::public.release_channel NOT NULL,
    auto_update boolean DEFAULT false NOT NULL,
    resolved_image_tag text DEFAULT ''::text NOT NULL,
    last_release_check_at timestamp with time zone
);

CREATE TABLE public.provisioning_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text,
    provider text NOT NULL,
    location text NOT NULL,
    datacenter text,
    server_type text NOT NULL,
    image text DEFAULT 'ubuntu-22.04'::text NOT NULL,
    server_count integer DEFAULT 1 NOT NULL,
    ipv4_per_server integer DEFAULT 1 NOT NULL,
    ipv6_per_server integer DEFAULT 1 NOT NULL,
    worker_profile_id uuid,
    tier text NOT NULL,
    egress_kind text DEFAULT 'cold_smtp'::text NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    placement_group text,
    private_network text,
    firewall text,
    is_auto_template boolean DEFAULT false NOT NULL,
    est_monthly_cost numeric(10,2),
    est_cost_currency text DEFAULT 'EUR'::text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT provisioning_templates_egress_kind_check CHECK ((egress_kind = ANY (ARRAY['cold_smtp'::text, 'oauth_api'::text, 'warmup_only'::text]))),
    CONSTRAINT provisioning_templates_ipv4_per_server_check CHECK (((ipv4_per_server >= 1) AND (ipv4_per_server <= 64))),
    CONSTRAINT provisioning_templates_server_count_check CHECK (((server_count >= 1) AND (server_count <= 100))),
    CONSTRAINT provisioning_templates_tier_check CHECK ((tier = ANY (ARRAY['shared_free'::text, 'shared_premium'::text, 'dedicated'::text])))
);

CREATE TABLE public.provisioning_policy (
    provider text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    auto_provision boolean DEFAULT false NOT NULL,
    max_per_day integer DEFAULT 2 NOT NULL,
    max_per_month integer DEFAULT 30 NOT NULL,
    monthly_budget numeric(10,2) DEFAULT 500,
    budget_currency text DEFAULT 'EUR'::text,
    cooldown_min integer DEFAULT 60 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.provisioning_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    triggered_by text NOT NULL,
    provider text NOT NULL,
    credential_id uuid,
    template_id uuid,
    config jsonb NOT NULL,
    provider_server_id text,
    provider_ip_ids text[],
    ips inet[],
    worker_ids uuid[],
    est_monthly_cost numeric(10,2),
    cost_currency text DEFAULT 'EUR'::text,
    error text,
    attempts integer DEFAULT 0 NOT NULL,
    last_step_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT provisioning_jobs_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'creating_server'::text, 'creating_ips'::text, 'assigning_ips'::text, 'setting_rdns'::text, 'installing'::text, 'verifying'::text, 'completed'::text, 'failed'::text, 'rolling_back'::text])))
);

ALTER TABLE ONLY public.aws_credentials ADD CONSTRAINT aws_credentials_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.aws_credentials ADD CONSTRAINT aws_credentials_name_key UNIQUE (name);
ALTER TABLE ONLY public.worker_profiles ADD CONSTRAINT worker_profiles_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.worker_profiles ADD CONSTRAINT worker_profiles_name_key UNIQUE (name);
ALTER TABLE ONLY public.provisioning_templates ADD CONSTRAINT provisioning_templates_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.provisioning_templates ADD CONSTRAINT provisioning_templates_name_key UNIQUE (name);
ALTER TABLE ONLY public.provisioning_policy ADD CONSTRAINT provisioning_policy_pkey PRIMARY KEY (provider);
ALTER TABLE ONLY public.provisioning_jobs ADD CONSTRAINT provisioning_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.provisioning_jobs
    ADD CONSTRAINT provisioning_jobs_template_id_fkey FOREIGN KEY (template_id) REFERENCES public.provisioning_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.provisioning_templates
    ADD CONSTRAINT provisioning_templates_worker_profile_id_fkey FOREIGN KEY (worker_profile_id) REFERENCES public.worker_profiles(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.worker_profiles
    ADD CONSTRAINT worker_profiles_aws_credential_id_fkey FOREIGN KEY (aws_credential_id) REFERENCES public.aws_credentials(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.workers
    ADD CONSTRAINT workers_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.worker_profiles(id) ON DELETE SET NULL;

COMMIT;
