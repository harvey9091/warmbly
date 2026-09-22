-- False reply data cannot be restored; only processing-claim columns are reversible.
ALTER TABLE unibox_emails
    DROP COLUMN campaign_reply_processed_at,
    DROP COLUMN campaign_reply_claim_token,
    DROP COLUMN campaign_reply_claimed_at;
