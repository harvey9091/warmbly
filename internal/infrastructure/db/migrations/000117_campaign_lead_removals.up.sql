-- A lead a user removed from a campaign by hand. Automatic segment enrolment
-- must not re-add these pairs; an explicit manual add clears the record.
CREATE TABLE campaign_lead_removals (
    campaign_id uuid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    contact_id uuid NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, contact_id)
);

CREATE INDEX idx_campaign_lead_removals_contact ON campaign_lead_removals (contact_id);
