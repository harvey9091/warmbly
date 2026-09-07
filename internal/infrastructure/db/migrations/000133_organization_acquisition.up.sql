-- Where a workspace came from, captured once at signup and never updated
-- (issue #352). First-party and deliberately minimal: the landing path, the
-- referring host and the UTM parameters the signup link carried.
--
-- It exists so revenue by channel is a SQL join against subscriptions rather
-- than a vendor report, and so the answer survives an analytics provider being
-- swapped, blocked or dropped. Every column is nullable because most signups
-- carry none of them: a direct visit to the dashboard has no landing page and
-- no campaign.
CREATE TABLE public.organization_acquisition (
    organization_id uuid PRIMARY KEY REFERENCES public.organizations(id) ON DELETE CASCADE,
    landing_path text,
    referrer_host text,
    utm_source text,
    utm_medium text,
    utm_campaign text,
    utm_term text,
    utm_content text,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

-- The one query this table exists for: signups grouped by channel.
CREATE INDEX idx_organization_acquisition_channel
    ON public.organization_acquisition (utm_source, utm_medium);

COMMENT ON TABLE public.organization_acquisition IS
    'Acquisition channel recorded once at signup. Never updated; a row is absent when the signup carried nothing.';
