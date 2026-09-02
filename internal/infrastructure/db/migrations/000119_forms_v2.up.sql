-- Forms v2: brand assets, personalized links and funnel events. Design v2
-- keys and page breaks are jsonb-additive and need no DDL.

ALTER TABLE forms
    ADD COLUMN logo_url  text NOT NULL DEFAULT '',
    ADD COLUMN cover_url text NOT NULL DEFAULT '';

-- A submission arriving through a personalized link knows the campaign whose
-- email carried that link.
ALTER TABLE form_submissions
    ADD COLUMN campaign_id uuid REFERENCES campaigns(id) ON DELETE SET NULL;

-- Per-contact form URL tickets, mirroring tracked_links: the row id IS the
-- token in ?t=, so no signing secret exists anywhere.
CREATE TABLE form_links (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    campaign_id uuid REFERENCES campaigns(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (form_id, contact_id)
);
CREATE INDEX idx_form_links_org ON form_links (organization_id);
CREATE INDEX idx_form_links_contact ON form_links (contact_id);

-- Funnel events, modeled on website_page_hits: enriched server-side from the
-- request IP (country only), the IP itself is never stored.
CREATE TABLE form_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    event_type text NOT NULL CHECK (event_type IN ('view','start','page','submit')),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    visitor_key text NOT NULL DEFAULT '',
    page_index integer NOT NULL DEFAULT 0,
    pages_total integer NOT NULL DEFAULT 0,
    contact_id uuid REFERENCES contacts(id) ON DELETE CASCADE,
    campaign_id uuid REFERENCES campaigns(id) ON DELETE SET NULL,
    referrer_domain text NOT NULL DEFAULT '',
    country_code text NOT NULL DEFAULT '',
    device text NOT NULL DEFAULT 'unknown'
        CHECK (device IN ('desktop','mobile','tablet','unknown'))
);
CREATE INDEX idx_form_events_form ON form_events (form_id, occurred_at DESC);
CREATE INDEX idx_form_events_org ON form_events (organization_id, occurred_at DESC);
CREATE INDEX idx_form_events_contact ON form_events (form_id, contact_id)
    WHERE contact_id IS NOT NULL;
