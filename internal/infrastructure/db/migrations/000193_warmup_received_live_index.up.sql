-- The retention sweep walks the live receipts oldest first; the retired ones
-- are the bulk and never need reading again. Built concurrently, on its own,
-- because warmup_received is written on every warmup arrival.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_warmup_received_live
    ON public.warmup_received (created_at)
    WHERE retired_at IS NULL;
