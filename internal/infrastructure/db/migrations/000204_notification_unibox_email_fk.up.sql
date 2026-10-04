-- NOT VALID and never validated: every existing row holds NULL, so a scan
-- would only confirm that. It is enforced, and cascades, for every new row.
ALTER TABLE notifications
    ADD CONSTRAINT notifications_unibox_email_id_fkey
    FOREIGN KEY (unibox_email_id) REFERENCES unibox_emails (id) ON DELETE CASCADE
    NOT VALID;

-- Reading a message reads its notification, whichever path read it: the
-- unibox, the API, or the customer's own mail client through sync.
CREATE FUNCTION public.notifications_read_with_message() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    -- An email-only notification is born read, so its pending email is
    -- cancelled whatever read_at says.
    UPDATE public.notifications
       SET read_at = COALESCE(read_at, now()),
           email_state = CASE WHEN email_state = 'pending' THEN 'skipped' ELSE email_state END,
           email_due_at = CASE WHEN email_state = 'pending' THEN NULL ELSE email_due_at END
     WHERE unibox_email_id = NEW.id AND (read_at IS NULL OR email_state = 'pending');
    RETURN NULL;
END;
$$;

CREATE TRIGGER notifications_read_with_message
    AFTER UPDATE OF seen ON public.unibox_emails
    FOR EACH ROW
    WHEN (NEW.seen AND NOT OLD.seen)
    EXECUTE FUNCTION public.notifications_read_with_message();
