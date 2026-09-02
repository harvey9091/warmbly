-- Live segment -> campaign links. Contacts matching an attached segment are
-- enrolled as campaign leads automatically; enrolment is additive, so a
-- contact falling out of the segment keeps their lead row.
CREATE TABLE campaign_segments (
    campaign_id uuid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    segment_id uuid NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, segment_id)
);

CREATE INDEX idx_campaign_segments_segment ON campaign_segments (segment_id);
