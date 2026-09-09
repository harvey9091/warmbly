-- Entry delay: how long a campaign waits before a contact's FIRST email, counted
-- from the moment that contact entered the campaign. 0 (the default) keeps the
-- existing behaviour, where a new lead is due the instant it is added.
ALTER TABLE campaigns ADD COLUMN entry_delay_minutes integer NOT NULL DEFAULT 0
    CHECK (entry_delay_minutes >= 0 AND entry_delay_minutes <= 129600); -- 90 days

-- When the contact entered the campaign; the anchor the delay counts from.
-- Deliberately nullable with no backfill: adding a NOT NULL column defaulted to
-- now() rewrites the whole table, and a NULL already has an honest reading —
-- the lead was in the campaign before this column existed, so the campaign's own
-- created_at is used instead (see loadRouter). New rows get now() from the default.
ALTER TABLE campaign_leads ADD COLUMN added_at timestamptz;
ALTER TABLE campaign_leads ALTER COLUMN added_at SET DEFAULT now();
