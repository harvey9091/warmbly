-- Alone in its file for CONCURRENTLY. Every unibox deletion and read probes it
-- through 000204, so it has to exist before that migration does.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_unibox_email
    ON notifications (unibox_email_id)
    WHERE unibox_email_id IS NOT NULL;
