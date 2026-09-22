ALTER TABLE email_tasks
    DROP COLUMN IF EXISTS click_count,
    DROP COLUMN IF EXISTS clicked_at,
    DROP COLUMN IF EXISTS opened_machine,
    DROP COLUMN IF EXISTS opened_at,
    DROP COLUMN IF EXISTS tracked;

ALTER TABLE email_accounts
    DROP COLUMN IF EXISTS track_direct_mail;
