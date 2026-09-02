ALTER TABLE organizations
    DROP COLUMN IF EXISTS forms_domain,
    DROP COLUMN IF EXISTS forms_domain_verified,
    DROP COLUMN IF EXISTS forms_domain_verified_at;
