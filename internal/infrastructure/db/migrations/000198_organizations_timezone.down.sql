ALTER TABLE campaigns ALTER COLUMN timezone SET DEFAULT 'Europe/London';
ALTER TABLE organizations DROP COLUMN IF EXISTS timezone;
