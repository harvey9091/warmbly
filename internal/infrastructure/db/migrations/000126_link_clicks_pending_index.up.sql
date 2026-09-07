-- The sweep for held-back click announcements reads only pending rows, so
-- the index is partial. Built concurrently, on its own, because the click
-- log is a live table and a plain CREATE INDEX would block writes to it.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_link_clicks_pending ON email_link_clicks (clicked_at) WHERE announce_pending;
