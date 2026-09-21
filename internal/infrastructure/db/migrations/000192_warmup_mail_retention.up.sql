-- Warmup mail is retained for a bounded time, by the platform.
--
-- Warmup mail is filed out of the way in the customer's mailbox, but it was
-- never removed: a mailbox on a fixed quota accumulated it indefinitely, and
-- the owner clearing the folder by hand was read as tampering. The platform
-- now deletes warmup mail from the mailbox after a retention window, and a
-- message it has retired can never be a strike against the mailbox.
--
-- email_accounts.warmup_retention_days: per-mailbox window, NULL meaning the
--   instance setting (retention.warmup_mail_days).
-- warmup_received.retired_at: when the platform sent the deletion for the
--   copy this mailbox received. Set before the worker acts, so a removal the
--   sync then observes is known to be ours.
-- warmup_tokens.sent_retired_at: the same for the sender's own copy of the
--   message, which is filed into the same folder.
ALTER TABLE public.email_accounts
    ADD COLUMN warmup_retention_days integer,
    ADD CONSTRAINT email_accounts_warmup_retention_days_check
        CHECK (warmup_retention_days IS NULL OR (warmup_retention_days >= 3 AND warmup_retention_days <= 3650));

ALTER TABLE public.warmup_received
    ADD COLUMN retired_at timestamp with time zone;

ALTER TABLE public.warmup_tokens
    ADD COLUMN sent_retired_at timestamp with time zone;

-- The retention sweep walks the live rows oldest first; the retired ones are
-- the bulk and never need reading again.
CREATE INDEX idx_warmup_received_live
    ON public.warmup_received (created_at)
    WHERE retired_at IS NULL;

CREATE INDEX idx_warmup_tokens_sent_live
    ON public.warmup_tokens (created_at)
    WHERE sent_retired_at IS NULL AND sent_message_id <> '';
