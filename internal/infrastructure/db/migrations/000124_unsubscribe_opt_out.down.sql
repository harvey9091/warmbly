DROP FUNCTION IF EXISTS recipient_suppressed(uuid, text);
DELETE FROM suppressed_recipients WHERE kind = 'domain';
ALTER TABLE suppressed_recipients DROP CONSTRAINT IF EXISTS suppressed_recipients_kind_check;
ALTER TABLE suppressed_recipients DROP COLUMN IF EXISTS kind;
ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS campaigns_unsubscribe_mode_check;
ALTER TABLE campaigns DROP COLUMN IF EXISTS unsubscribe_mode;
