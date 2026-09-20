-- Read state reached the platform on every message as the \Seen flag, but the
-- column the inbox reads was stored false for every synced message. That made
-- a mailbox's own sent copies (filed in Sent flagged \Seen, since the sender
-- wrote them) arrive unread, and left mail already read in the customer's own
-- client unread here. The flag array recorded the truth all along, so adopt it
-- for rows stored before the sync started writing the column.
UPDATE public.unibox_emails
SET seen = TRUE,
    updated_at = NOW()
WHERE seen = FALSE
  AND flags @> ARRAY['\Seen']::text[];
