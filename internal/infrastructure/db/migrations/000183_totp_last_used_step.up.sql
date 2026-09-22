-- Remember the last TOTP step a user spent, so a code cannot be used twice.
--
-- A TOTP code is valid for its own 30-second step plus one on each side, so the
-- same six digits were accepted for up to 90 seconds. Anyone who saw the code in
-- that window (over someone's shoulder, in a phishing relay, in a screen
-- recording) could replay it. RFC 6238 section 5.2 says the verifier must
-- refuse a second use of the same step, and this column is what lets it.

ALTER TABLE public.user_totp_settings
    ADD COLUMN IF NOT EXISTS last_used_step bigint NOT NULL DEFAULT 0;

COMMENT ON COLUMN public.user_totp_settings.last_used_step IS
    'Highest TOTP time step already accepted for this user. A code at or below it is a replay and is refused.';
