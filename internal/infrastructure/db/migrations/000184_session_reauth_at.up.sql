-- Record when a session last re-proved the person behind it.
--
-- CASA 2.4.1 wants a sensitive account change to sit behind a full session plus
-- re-authentication or a secondary check. Changing a password and disabling 2FA
-- already ask for a current credential; adding a passkey, minting an API key,
-- transferring a workspace and scheduling a deletion asked for nothing beyond a
-- live token. A stolen token is enough for all of those, and the first two hand
-- the attacker a durable credential of their own.
--
-- NULL means "never re-authenticated in this session", which is the state every
-- existing session starts in.

ALTER TABLE public.sessions
    ADD COLUMN IF NOT EXISTS reauth_at timestamptz;

COMMENT ON COLUMN public.sessions.reauth_at IS
    'When this session last re-proved the account holder (password, TOTP, recovery code or passkey). Read by RequireFreshAuth.';
