-- "none" is the loopback-only mode the connect paths already accept (a mail
-- server on the worker's own machine, such as Proton Bridge); the columns take it too.
ALTER TABLE email_accounts_smtp_imap
    DROP CONSTRAINT IF EXISTS email_accounts_smtp_imap_smtp_security_check,
    DROP CONSTRAINT IF EXISTS email_accounts_smtp_imap_imap_security_check;

ALTER TABLE email_accounts_smtp_imap
    ADD CONSTRAINT email_accounts_smtp_imap_smtp_security_check
    CHECK (smtp_security IN ('tls', 'starttls', 'none'));

ALTER TABLE email_accounts_smtp_imap
    ADD CONSTRAINT email_accounts_smtp_imap_imap_security_check
    CHECK (imap_security IN ('tls', 'starttls', 'none'));
