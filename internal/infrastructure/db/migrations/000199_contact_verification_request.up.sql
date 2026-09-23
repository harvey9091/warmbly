-- A queued re-verify beside the verdict it replaces, and what the last check
-- itself said before real mail was weighed in ('' for older verdicts).
ALTER TABLE public.contacts
    ADD COLUMN IF NOT EXISTS verification_requested_at timestamptz,
    ADD COLUMN IF NOT EXISTS verification_check_status text NOT NULL DEFAULT '';

-- Every existing row holds the default, so validating would only buy a full
-- scan under ACCESS EXCLUSIVE; NOT VALID is still enforced on every write.
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_verification_check_status_check
    CHECK (verification_check_status IN ('', 'valid', 'risky', 'invalid', 'unknown')) NOT VALID;
