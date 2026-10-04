ALTER TABLE public.contacts
    DROP CONSTRAINT IF EXISTS contacts_verification_check_status_check;

ALTER TABLE public.contacts
    DROP COLUMN IF EXISTS verification_check_status,
    DROP COLUMN IF EXISTS verification_requested_at;
