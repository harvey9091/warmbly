-- The same for the sender copies the retention sweep walks. Built
-- concurrently, on its own, because warmup_tokens is written on every send.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_warmup_tokens_sent_live
    ON public.warmup_tokens (created_at)
    WHERE sent_retired_at IS NULL AND sent_message_id <> '';
