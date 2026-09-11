-- One unresolved error row per mailbox per code.
--
-- The sync loop retries a refused mail server about once a minute and relays
-- what it got every time, and the consumer recorded each one, so a mailbox
-- whose server kept saying no collected an identical row a minute for as long
-- as it lasted (issue #405). The insert is conditional now, but a conditional
-- insert is not atomic on its own: two workers relaying the same failure at
-- the same moment both see no row and both write one.
--
-- The unique index is what actually holds. It is partial on the unresolved
-- rows, so the history of resolved errors is untouched and a problem that
-- comes back after it was fixed is still recorded as news.

BEGIN;

-- Existing duplicates first, or the index cannot be built. The oldest of each
-- group is kept because it says when the problem started; the rest are marked
-- resolved rather than deleted, so the record of how long it went on survives.
UPDATE email_account_errors AS e
SET resolved_at = NOW(),
    resolved_by = 'superseded'
WHERE e.resolved_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM email_account_errors AS keep
      WHERE keep.email_account_id = e.email_account_id
        AND keep.error_code = e.error_code
        AND keep.resolved_at IS NULL
        AND (keep.created_at, keep.id) < (e.created_at, e.id)
  );

CREATE UNIQUE INDEX idx_email_account_errors_one_unresolved_per_code
    ON email_account_errors (email_account_id, error_code)
    WHERE resolved_at IS NULL;

-- Same columns, same predicate, no longer unique: the index above serves every
-- lookup this one did, and a redundant index is paid for on every write.
DROP INDEX IF EXISTS idx_email_account_errors_code;

COMMIT;
