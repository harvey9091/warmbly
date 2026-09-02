-- Organization-level custom forms domain. A customer points a subdomain of
-- the domain they send from at this install's forms host, so hosted form
-- links carry their own name instead of the shared one. Only a verified
-- domain is ever used to build a URL: an unresolved host would make every
-- form link in an email dead, so it falls back to the shared host instead.
ALTER TABLE organizations
    ADD COLUMN forms_domain text NOT NULL DEFAULT '',
    ADD COLUMN forms_domain_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN forms_domain_verified_at timestamptz;
