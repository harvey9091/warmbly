-- Per-link click attribution and automatic UTM tagging.
--
-- Every click on a tracked link is logged as its own row so the contact
-- timeline can say WHICH link was clicked, not only that one was. A click
-- that looks automated (a security gateway following every link seconds
-- after delivery) is kept for the record but flagged, and it never stamps
-- campaign_contact_progress.clicked_at, so "clicked" keeps meaning a person.

ALTER TABLE campaigns
    ADD COLUMN IF NOT EXISTS utm_tracking boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS utm_source text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS utm_medium text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS utm_campaign text NOT NULL DEFAULT '';

-- The anchor text the link was minted from ("Pricing"), so a click can be
-- named without re-parsing the email.
ALTER TABLE tracked_links
    ADD COLUMN IF NOT EXISTS label text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS email_link_clicks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tracked_link_id uuid REFERENCES tracked_links(id) ON DELETE SET NULL,
    task_id uuid NOT NULL,
    campaign_id uuid NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    sequence_id uuid NOT NULL REFERENCES sequences(id) ON DELETE CASCADE,
    destination text NOT NULL,
    label text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    ip_hash text NOT NULL DEFAULT '',
    machine boolean NOT NULL DEFAULT false,
    machine_reason text NOT NULL DEFAULT '' CHECK (machine_reason IN ('', 'prefetch', 'instant', 'burst')),
    clicked_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_link_clicks_contact ON email_link_clicks (contact_id, clicked_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_link_clicks_task ON email_link_clicks (task_id, clicked_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_link_clicks_step ON email_link_clicks (campaign_id, contact_id, sequence_id);
