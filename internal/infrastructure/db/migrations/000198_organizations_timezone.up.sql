-- The workspace timezone: what a campaign or mailbox with no timezone of its own
-- follows. Empty means not set, which reads as UTC.
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT '';

-- A campaign's timezone is optional from here on: empty follows the workspace,
-- resolved on every read, the way an empty mailbox timezone already does.
-- Existing rows keep the zone they were created with.
ALTER TABLE campaigns ALTER COLUMN timezone SET DEFAULT '';
