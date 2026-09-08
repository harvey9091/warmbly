-- The contacts list and every campaign's Leads tab page through one ordering:
-- an organization's contacts newest first, with the row id breaking ties
-- (issue #382). Without this the keyset has to read the whole organization on
-- every page. Descending on both columns so a forward scan serves the default
-- order and a backward scan serves the reversed one. Built concurrently, on its
-- own, because contacts is a live table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_org_created
    ON contacts (organization_id, created_at DESC, id DESC);
