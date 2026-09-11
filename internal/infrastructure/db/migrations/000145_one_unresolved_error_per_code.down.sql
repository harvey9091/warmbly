-- Reverses 000145. The rows the up migration collapsed are recoverable
-- because it stamped them with a resolved_by nothing else writes.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_email_account_errors_code
    ON email_account_errors (email_account_id, error_code)
    WHERE resolved_at IS NULL;

DROP INDEX IF EXISTS idx_email_account_errors_one_unresolved_per_code;

UPDATE email_account_errors
SET resolved_at = NULL,
    resolved_by = NULL
WHERE resolved_by = 'superseded';

COMMIT;
