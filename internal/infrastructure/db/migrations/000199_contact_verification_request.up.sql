-- A member's re-verify is recorded beside the verdict it replaces, so the
-- current verdict (and every campaign routing on it) stands until the new
-- check lands. verification_check_status is what that check itself said,
-- before real mail to the address was weighed against it; '' reads as
-- verification_status for verdicts stored before it existed.
ALTER TABLE public.contacts
    ADD COLUMN IF NOT EXISTS verification_requested_at timestamptz,
    ADD COLUMN IF NOT EXISTS verification_check_status text NOT NULL DEFAULT '';

-- Every existing row holds the default, so validating would only buy a full
-- scan under ACCESS EXCLUSIVE; NOT VALID is still enforced on every write.
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_verification_check_status_check
    CHECK (verification_check_status IN ('', 'valid', 'risky', 'invalid', 'unknown')) NOT VALID;
