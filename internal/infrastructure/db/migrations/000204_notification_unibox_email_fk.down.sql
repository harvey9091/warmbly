DROP TRIGGER IF EXISTS notifications_read_with_message ON public.unibox_emails;
DROP FUNCTION IF EXISTS public.notifications_read_with_message();
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_unibox_email_id_fkey;
