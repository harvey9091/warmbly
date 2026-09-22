-- Sign-in, password reset and just-in-time SSO provisioning all match
-- `users.email` exactly, and `users_email_key` is a plain unique index on the
-- raw column. A row written with the case someone typed is therefore a row no
-- later lookup finds: the reset flow answers 200 for an unknown address by
-- design, so the person was told the mail was sent and nothing arrived.
--
-- The application now folds on both read and write. This brings the rows that
-- predate it into the same form.
--
-- Rows whose folded form is already taken by another account are deliberately
-- left alone: the UPDATE would violate users_email_key and fail the migration,
-- which on a self-hosted instance means the backend restart-loops at boot. An
-- instance holding a genuine pair like that needs a human to decide which
-- account survives, not a migration guessing.
UPDATE users u
   SET email = lower(btrim(u.email)),
       updated_at = now()
 WHERE u.email <> lower(btrim(u.email))
   AND NOT EXISTS (
       SELECT 1 FROM users o
        WHERE o.id <> u.id
          AND o.email = lower(btrim(u.email))
   );
