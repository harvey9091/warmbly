-- Member-requested checks are picked before the backlog, through this index.
-- Built concurrently, on its own, because contacts is a live table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_verification_requested
    ON contacts (verification_requested_at)
    WHERE verification_requested_at IS NOT NULL;
