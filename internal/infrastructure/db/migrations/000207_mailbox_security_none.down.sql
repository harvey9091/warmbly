-- The narrower check cannot hold a "none" row; those fall back to the port's usual mode.
UPDATE email_accounts_smtp_imap
SET smtp_security = CASE WHEN smtp_port = 465 THEN 'tls' ELSE 'starttls' END
WHERE smtp_security = 'none';

UPDATE email_accounts_smtp_imap
SET imap_security = CASE WHEN imap_port = 143 THEN 'starttls' ELSE 'tls' END
WHERE imap_security = 'none';

ALTER TABLE email_accounts_smtp_imap
    DROP CONSTRAINT IF EXISTS email_accounts_smtp_imap_smtp_security_check,
    DROP CONSTRAINT IF EXISTS email_accounts_smtp_imap_imap_security_check;

ALTER TABLE email_accounts_smtp_imap
    ADD CONSTRAINT email_accounts_smtp_imap_smtp_security_check
    CHECK (smtp_security IN ('tls', 'starttls'));

ALTER TABLE email_accounts_smtp_imap
    ADD CONSTRAINT email_accounts_smtp_imap_imap_security_check
    CHECK (imap_security IN ('tls', 'starttls'));
