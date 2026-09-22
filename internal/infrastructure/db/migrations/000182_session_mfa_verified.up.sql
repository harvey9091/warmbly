-- Record whether a session proved a second factor.
--
-- Multi-factor authentication existed but nothing could tell afterwards whether
-- a given session had used it: auth_provider says "email" whether the sign-in
-- was password-only or password plus a TOTP code. Without that distinction the
-- admin panel could not require MFA, which CASA 3.3.1 asks for on any
-- internet-reachable administrative interface.
--
-- Existing sessions default to false, so an admin signed in right now is asked
-- to sign in again with their second factor. That is the intended direction:
-- the safe default for "we do not know" is "not verified".

ALTER TABLE public.sessions
    ADD COLUMN IF NOT EXISTS mfa_verified boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN public.sessions.mfa_verified IS
    'True when this session was established with a second factor: a TOTP code, a recovery code, or a passkey (which is itself multi-factor).';
