-- Hosted lead-capture forms (issue #267). The field list and the design theme
-- are read-then-execute blobs edited as a whole in the builder, so they live
-- as jsonb validated at the app boundary; everything queried, counted or
-- enforced in SQL is a typed column.
CREATE TABLE public.forms (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    created_by uuid REFERENCES public.users(id) ON DELETE SET NULL,
    public_id text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'draft',
    fields jsonb NOT NULL DEFAULT '[]'::jsonb,
    design jsonb NOT NULL DEFAULT '{}'::jsonb,
    success_message text NOT NULL DEFAULT 'Thanks! Your submission has been received.',
    redirect_url text NOT NULL DEFAULT '',
    campaign_id uuid REFERENCES public.campaigns(id) ON DELETE SET NULL,
    allowed_domains text[] NOT NULL DEFAULT '{}',
    captcha_enabled boolean NOT NULL DEFAULT false,
    views_count bigint NOT NULL DEFAULT 0,
    submissions_count bigint NOT NULL DEFAULT 0,
    last_submission_at timestamp with time zone,
    published_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT forms_status_check CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT forms_fields_check CHECK (jsonb_typeof(fields) = 'array'),
    CONSTRAINT forms_design_check CHECK (jsonb_typeof(design) = 'object')
);

CREATE UNIQUE INDEX forms_public_id_unique ON public.forms (public_id);
CREATE INDEX idx_forms_org ON public.forms (organization_id);

COMMENT ON COLUMN public.forms.fields IS
    'Ordered field list, validated at the app boundary (models.FormField).';
COMMENT ON COLUMN public.forms.design IS
    'Theme overrides, validated at the app boundary (models.FormDesign).';

-- Categories a submission files the contact under (the "list/tag" of #267).
CREATE TABLE public.form_categories (
    form_id uuid NOT NULL REFERENCES public.forms(id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES public.categories(id) ON DELETE CASCADE,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (form_id, category_id)
);

CREATE INDEX idx_form_categories_category ON public.form_categories (category_id);

CREATE TABLE public.form_submissions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    form_id uuid NOT NULL REFERENCES public.forms(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    contact_id uuid REFERENCES public.contacts(id) ON DELETE SET NULL,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_url text NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT form_submissions_data_check CHECK (jsonb_typeof(data) = 'object')
);

CREATE INDEX idx_form_submissions_form ON public.form_submissions (form_id, created_at DESC);
CREATE INDEX idx_form_submissions_org ON public.form_submissions (organization_id);
CREATE INDEX idx_form_submissions_contact ON public.form_submissions (contact_id);

-- A submission is a first-touch origin and a timeline event of its own.
ALTER TABLE public.contacts DROP CONSTRAINT contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant', 'form'));

ALTER TYPE public.activity_type ADD VALUE IF NOT EXISTS 'form_submitted';
