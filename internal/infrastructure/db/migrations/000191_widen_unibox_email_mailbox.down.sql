-- Narrowing back fails on any row that needed the extra range, which is the
-- point of the change. Values that fit are unaffected.
ALTER TABLE unibox_emails ALTER COLUMN mailbox TYPE integer;
