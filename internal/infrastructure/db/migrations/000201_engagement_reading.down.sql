-- Scanner-labelled rows written since the up migration stay; the narrower
-- checks go back NOT VALID so they do not refuse them.
ALTER TABLE email_link_clicks
    DROP CONSTRAINT email_link_clicks_machine_reason_check,
    ADD CONSTRAINT email_link_clicks_machine_reason_check
        CHECK (machine_reason IN ('', 'prefetch', 'instant', 'burst')) NOT VALID,
    DROP CONSTRAINT email_link_clicks_client_type_check,
    DROP COLUMN device_hidden,
    DROP COLUMN client_type;

ALTER TABLE email_opens
    DROP CONSTRAINT email_opens_machine_reason_check,
    ADD CONSTRAINT email_opens_machine_reason_check
        CHECK (machine_reason IN ('', 'prefetch', 'instant')) NOT VALID,
    DROP CONSTRAINT email_opens_client_type_check,
    DROP COLUMN device_hidden,
    DROP COLUMN client_type;
