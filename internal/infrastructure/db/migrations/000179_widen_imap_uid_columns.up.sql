-- RFC 3501 defines UIDVALIDITY and UID as unsigned 32-bit values, and the Go
-- models carry them as uint32. These two columns are int4, so every value at
-- or above 2^31 fails to bind at all:
--
--   unable to encode 0x85d5efd6 into binary format for int4 (OID 23):
--   2245390294 is greater than maximum value for int4
--
-- A server that stamps UIDVALIDITY with anything other than a Unix timestamp
-- reaches that range routinely, and when it does the folder row can never be
-- written, so that mailbox's folder is never stored and its sync never
-- progresses. Nothing is out of range yet, which is why widening is enough and
-- no repair is needed.
--
-- uid_next and highestmodseq on the same table are already bigint for exactly
-- this reason; these two were missed.
ALTER TABLE unibox_mailboxes ALTER COLUMN uid_validity TYPE bigint;
ALTER TABLE unibox_emails ALTER COLUMN uid TYPE bigint;
