ALTER TABLE campaigns ALTER COLUMN timezone SET DEFAULT 'Europe/London';
UPDATE campaigns c SET timezone = COALESCE(NULLIF(o.timezone, ''), 'UTC')
FROM organizations o WHERE c.organization_id = o.id AND c.timezone = '';
UPDATE campaigns SET timezone = 'UTC' WHERE timezone = '';
UPDATE email_accounts ea SET timezone = o.timezone
FROM organizations o WHERE ea.organization_id = o.id AND ea.timezone = '' AND o.timezone <> '';
ALTER TABLE organizations DROP COLUMN IF EXISTS timezone;
