-- Restoring the instance-wide constraint can fail where two organizations
-- legitimately hold the same key, which is the state the up migration allows.
-- Duplicates are collapsed to the oldest row first so the rollback applies.

ALTER TABLE public.deliverability_events
    DROP CONSTRAINT IF EXISTS deliverability_events_idempotency_unique;

DELETE FROM public.deliverability_events a
    USING public.deliverability_events b
    WHERE a.idempotency_key = b.idempotency_key
      AND a.ctid > b.ctid;

ALTER TABLE public.deliverability_events
    ADD CONSTRAINT deliverability_events_idempotency_unique
    UNIQUE (idempotency_key);
