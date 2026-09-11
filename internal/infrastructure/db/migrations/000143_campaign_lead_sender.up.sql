-- A lead's sending mailbox is fixed for the whole sequence. Rotation picks the
-- mailbox for a lead's FIRST email and that mailbox sends every follow-up, so a
-- contact never hears from three different addresses in one conversation and
-- their replies always land in the mailbox that wrote to them (issue #401).
-- Rotation still spreads NEW leads across the pool.
--
-- Nullable: a lead has no sender until its first email is reserved. ON DELETE
-- SET NULL because a disconnected mailbox must not take its leads with it — the
-- scheduler moves them to another mailbox on their next step.
ALTER TABLE campaign_leads
    ADD COLUMN email_account_id uuid REFERENCES email_accounts (id) ON DELETE SET NULL,
    ADD COLUMN sender_assigned_at timestamptz;

-- Which mailbox a lead was last actually sent from. The backfill below reads
-- it, and so does the scheduler when a lead has steps but no binding (removed
-- from the campaign and added back keeps the steps, not the row).
CREATE INDEX IF NOT EXISTS idx_campaign_tasks_lead
    ON campaign_tasks (campaign_id, contact_id, task_id)
    WHERE campaign_id IS NOT NULL AND contact_id IS NOT NULL;

-- Leads already mid-sequence keep the address they have been writing from. The
-- earliest completed campaign task for the pair is the mailbox the contact saw
-- first; a mailbox that has since been deleted leaves the lead unassigned, and
-- its next step picks one by rotation exactly as a new lead does.
WITH first_send AS (
    SELECT DISTINCT ON (ct.campaign_id, ct.contact_id)
           ct.campaign_id, ct.contact_id, t.email_account_id, t.created_at
    FROM campaign_tasks ct
    JOIN tasks t ON t.id = ct.task_id
    WHERE ct.campaign_id IS NOT NULL
      AND ct.contact_id IS NOT NULL
      AND t.task_type = 'campaign'
      AND t.status = 'completed'
    ORDER BY ct.campaign_id, ct.contact_id, t.created_at ASC
)
UPDATE campaign_leads cl
SET email_account_id   = fs.email_account_id,
    sender_assigned_at = fs.created_at
FROM first_send fs
WHERE cl.campaign_id = fs.campaign_id
  AND cl.contact_id = fs.contact_id
  AND EXISTS (SELECT 1 FROM email_accounts ea WHERE ea.id = fs.email_account_id);

-- Read by the mailbox drawer's "leads pinned here" count and by the FK's own
-- ON DELETE SET NULL sweep.
CREATE INDEX IF NOT EXISTS idx_campaign_leads_sender
    ON campaign_leads (email_account_id) WHERE email_account_id IS NOT NULL;
