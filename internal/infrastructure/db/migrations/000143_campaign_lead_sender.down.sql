DROP INDEX IF EXISTS idx_campaign_leads_sender;
DROP INDEX IF EXISTS idx_campaign_tasks_lead;

ALTER TABLE campaign_leads
    DROP COLUMN IF EXISTS email_account_id,
    DROP COLUMN IF EXISTS sender_assigned_at;
