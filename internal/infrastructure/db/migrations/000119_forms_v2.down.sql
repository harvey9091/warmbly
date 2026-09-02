DROP TABLE IF EXISTS form_events;
DROP TABLE IF EXISTS form_links;
ALTER TABLE form_submissions
    DROP COLUMN IF EXISTS campaign_id;
ALTER TABLE forms
    DROP COLUMN IF EXISTS logo_url,
    DROP COLUMN IF EXISTS cover_url;
