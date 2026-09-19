-- Scope the deliverability idempotency key to the organization.
--
-- The key was unique instance-wide, and the keys the platform itself mints are
-- derived from ids the recipient of a campaign email can read ("reject:<taskID>",
-- "ndr:<messageID>", "fbl:<messageID>"). One workspace could therefore insert a
-- row carrying another workspace's future key and, because the writer is
-- ON CONFLICT DO NOTHING, silently discard that workspace's real bounce or
-- complaint for good.
--
-- Rows are deduplicated before the new index is built: an existing collision
-- across two organizations is exactly the case the old constraint could not
-- represent, and the oldest row is the one that was recorded first.

ALTER TABLE public.deliverability_events
    DROP CONSTRAINT IF EXISTS deliverability_events_idempotency_unique;

DELETE FROM public.deliverability_events a
    USING public.deliverability_events b
    WHERE a.organization_id = b.organization_id
      AND a.idempotency_key = b.idempotency_key
      AND a.ctid > b.ctid;

ALTER TABLE public.deliverability_events
    ADD CONSTRAINT deliverability_events_idempotency_unique
    UNIQUE (organization_id, idempotency_key);
