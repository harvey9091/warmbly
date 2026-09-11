-- Caddy's on-demand TLS `ask` probe resolves one question per unfamiliar SNI:
-- is this hostname a verified custom domain on this instance? Every TLS
-- handshake for a name with no certificate reaches it, including handshakes
-- from anyone who points a name at this machine, so the lookup has to be an
-- index hit rather than three sequential scans.
--
-- Partial on verified: an unverified domain never appears in mail, so it is
-- never asked about, and excluding it keeps the indexes the size of the set
-- that is actually reachable.

CREATE INDEX IF NOT EXISTS idx_email_accounts_tracking_domain_verified
    ON public.email_accounts (tracking_domain)
    WHERE tracking_domain_verified AND tracking_domain <> '';

CREATE INDEX IF NOT EXISTS idx_campaigns_tracking_domain_verified
    ON public.campaigns (tracking_domain)
    WHERE tracking_domain_verified AND tracking_domain <> '';

CREATE INDEX IF NOT EXISTS idx_organizations_forms_domain_verified
    ON public.organizations (forms_domain)
    WHERE forms_domain_verified AND forms_domain <> '';
