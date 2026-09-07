DROP TABLE IF EXISTS email_link_clicks;

ALTER TABLE tracked_links
    DROP COLUMN IF EXISTS label;

ALTER TABLE campaigns
    DROP COLUMN IF EXISTS utm_tracking,
    DROP COLUMN IF EXISTS utm_source,
    DROP COLUMN IF EXISTS utm_medium,
    DROP COLUMN IF EXISTS utm_campaign;
