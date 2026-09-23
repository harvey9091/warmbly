-- How an open was read: client_type says whether the mail client named by the
-- user agent is an installed app or webmail in a browser, and device_hidden
-- marks a fetch made by a mailbox provider's image proxy (Gmail, Yahoo, Apple
-- Mail Privacy Protection), whose device and network belong to the proxy
-- rather than the reader. The machine_reason checks gain 'scanner', the reason
-- the consumer records for a fetch from a known mail-filtering network.
-- Every check is NOT VALID: existing rows already satisfy it, and it is still
-- enforced on every write.
ALTER TABLE email_opens
    ADD COLUMN client_type text NOT NULL DEFAULT '',
    ADD COLUMN device_hidden boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT email_opens_client_type_check
        CHECK (client_type IN ('', 'app', 'webmail')) NOT VALID,
    DROP CONSTRAINT email_opens_machine_reason_check,
    ADD CONSTRAINT email_opens_machine_reason_check
        CHECK (machine_reason IN ('', 'prefetch', 'instant', 'scanner')) NOT VALID;

ALTER TABLE email_link_clicks
    ADD COLUMN client_type text NOT NULL DEFAULT '',
    ADD COLUMN device_hidden boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT email_link_clicks_client_type_check
        CHECK (client_type IN ('', 'app', 'webmail')) NOT VALID,
    DROP CONSTRAINT email_link_clicks_machine_reason_check,
    ADD CONSTRAINT email_link_clicks_machine_reason_check
        CHECK (machine_reason IN ('', 'prefetch', 'instant', 'burst', 'scanner')) NOT VALID;
