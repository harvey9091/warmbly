-- Repair real replies skipped when their thread root came from another workspace mailbox.

CREATE TEMP TABLE issue_549_missed_replies AS
WITH task_matches AS (
    SELECT
        ue.id AS source_message_id,
        ue.campaign_reply_processed_at AS replied_at,
        ue.subject,
        campaign_task.task_id,
        campaign_task.campaign_id,
        campaign_task.contact_id,
        campaign_task.sequence_id,
        campaign.organization_id,
        contact.email AS contact_email,
        count(*) OVER (PARTITION BY ue.id) AS task_match_count
    FROM unibox_emails ue
    JOIN email_accounts receiving_account
      ON receiving_account.id = ue.email_id
    JOIN tasks parent_task
      ON parent_task.task_type = 'campaign'
     AND parent_task.email_account_id <> ue.email_id
     AND EXISTS (
            SELECT 1
            FROM unnest(ue.in_reply_to) AS refs(message_id)
            WHERE btrim(refs.message_id, '<> ') = btrim(parent_task.message_id, '<> ')
        )
    JOIN campaign_tasks campaign_task
      ON campaign_task.task_id = parent_task.id
     AND campaign_task.campaign_id IS NOT NULL
     AND campaign_task.contact_id IS NOT NULL
     AND campaign_task.sequence_id IS NOT NULL
    JOIN campaigns campaign
      ON campaign.id = campaign_task.campaign_id
     AND campaign.organization_id = receiving_account.organization_id
    JOIN email_accounts sending_account
      ON sending_account.id = parent_task.email_account_id
     AND sending_account.organization_id = campaign.organization_id
    JOIN contacts contact
      ON contact.id = campaign_task.contact_id
     AND contact.organization_id = campaign.organization_id
     AND EXISTS (
            SELECT 1
            FROM unnest(ue.from_addr) AS senders(address)
            WHERE lower(btrim(COALESCE(
                substring(senders.address FROM '<([^>]*)>'),
                substring(senders.address FROM '\(([^()]*)\)\s*$'),
                senders.address
            ))) = lower(btrim(contact.email))
        )
    JOIN campaign_contact_progress progress
      ON progress.campaign_id = campaign_task.campaign_id
     AND progress.contact_id = campaign_task.contact_id
     AND progress.sequence_id = campaign_task.sequence_id
     AND progress.replied_at IS NULL
    WHERE ue.campaign_reply_processed_at IS NOT NULL
      AND ue.folder NOT IN ('sent', 'drafts')
      AND ue.provider_folder NOT IN ('sent', 'drafts')
      AND EXISTS (
            SELECT 1
            FROM campaign_tasks receiving_campaign_task
            JOIN tasks receiving_task ON receiving_task.id = receiving_campaign_task.task_id
            WHERE receiving_campaign_task.campaign_id = campaign_task.campaign_id
              AND receiving_campaign_task.contact_id = campaign_task.contact_id
              AND receiving_task.task_type = 'campaign'
              AND receiving_task.status = 'completed'
              AND receiving_task.email_account_id = ue.email_id
        )
      AND EXISTS (
            SELECT 1
            FROM unnest(ue.to_addr || ue.cc || ue.bcc) AS recipients(address)
            WHERE lower(btrim(COALESCE(
                substring(recipients.address FROM '<([^>]*)>'),
                substring(recipients.address FROM '\(([^()]*)\)\s*$'),
                recipients.address
            ))) IN (
                lower(receiving_account.email),
                lower(COALESCE(NULLIF(receiving_account.send_as_email, ''), receiving_account.email)),
                lower(COALESCE(NULLIF(receiving_account.reply_to, ''), receiving_account.email))
            )
        )
), intent_matches AS (
    SELECT
        task_matches.*,
        intent.id AS intent_id,
        intent.intent,
        intent.metadata,
        count(*) OVER (PARTITION BY task_matches.source_message_id) AS intent_match_count,
        count(*) OVER (PARTITION BY intent.id) AS source_match_count
    FROM task_matches
    JOIN reply_intents intent
      ON intent.organization_id = task_matches.organization_id
     AND intent.campaign_id IS NULL
     AND intent.task_id IS NULL
     AND lower(btrim(intent.contact_email)) = lower(btrim(task_matches.contact_email))
     AND COALESCE(intent.metadata->>'subject', '') = task_matches.subject
     AND intent.created_at BETWEEN task_matches.replied_at - INTERVAL '30 seconds'
                               AND task_matches.replied_at + INTERVAL '30 seconds'
    WHERE task_matches.task_match_count = 1
)
SELECT
    source_message_id,
    replied_at,
    task_id,
    campaign_id,
    contact_id,
    sequence_id,
    intent_id,
    metadata
FROM intent_matches
WHERE intent_match_count = 1
  AND source_match_count = 1
  AND intent NOT IN ('out_of_office', 'automated')
  AND COALESCE(metadata->>'reply_class', '') NOT IN ('auto_reply', 'out_of_office');

UPDATE campaign_contact_progress progress
SET replied_at = repair.replied_at,
    reply_class = CASE
        WHEN COALESCE(repair.metadata->>'reply_class', '') IN (
            'positive', 'negative', 'neutral', 'unsubscribe', 'unknown'
        ) THEN repair.metadata->>'reply_class'
        ELSE 'unknown'
    END,
    reply_confidence = 0,
    reply_source = CASE
        WHEN COALESCE(repair.metadata->>'classified_by', '') IN ('header', 'lexicon', 'model')
            THEN repair.metadata->>'classified_by'
        ELSE ''
    END
FROM issue_549_missed_replies repair
WHERE progress.campaign_id = repair.campaign_id
  AND progress.contact_id = repair.contact_id
  AND progress.sequence_id = repair.sequence_id
  AND progress.replied_at IS NULL;

UPDATE reply_intents intent
SET campaign_id = repair.campaign_id,
    task_id = repair.task_id
FROM issue_549_missed_replies repair
WHERE intent.id = repair.intent_id
  AND intent.campaign_id IS NULL
  AND intent.task_id IS NULL;

UPDATE campaign_ab_assignments assignment
SET replied_at = COALESCE(assignment.replied_at, repair.replied_at)
FROM issue_549_missed_replies repair
WHERE assignment.campaign_id = repair.campaign_id
  AND assignment.contact_id = repair.contact_id;

INSERT INTO contact_verification_evidence (contact_id, kind, ref, observed_at)
SELECT repair.contact_id, 'replied', repair.source_message_id::text, repair.replied_at
FROM issue_549_missed_replies repair
JOIN contacts contact ON contact.id = repair.contact_id
JOIN campaign_contact_progress progress
  ON progress.campaign_id = repair.campaign_id
 AND progress.contact_id = repair.contact_id
 AND progress.sequence_id = repair.sequence_id
WHERE contact.verification_evidence_reset_at IS NULL
   OR COALESCE(progress.dispatched_at, progress.sent_at) > contact.verification_evidence_reset_at
ON CONFLICT (contact_id, kind, ref) DO NOTHING;

WITH affected AS (
    SELECT DISTINCT contact_id
    FROM issue_549_missed_replies
), newest_evidence AS (
    SELECT
        evidence.contact_id,
        evidence.kind,
        evidence.observed_at,
        row_number() OVER (
            PARTITION BY evidence.contact_id
            ORDER BY evidence.observed_at DESC
        ) AS overall_rank
    FROM contact_verification_evidence evidence
    JOIN affected ON affected.contact_id = evidence.contact_id
), ranked_evidence AS (
    SELECT
        contact_id,
        kind,
        observed_at,
        row_number() OVER (
            PARTITION BY contact_id, kind
            ORDER BY observed_at DESC
        ) AS kind_rank
    FROM newest_evidence
    WHERE overall_rank <= 50
), remaining AS (
    SELECT
        affected.contact_id,
        COALESCE(sum(
            CASE
                WHEN evidence.kind = 'replied' AND evidence.kind_rank <= 3
                    THEN 45 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '365 days'))
                WHEN evidence.kind = 'auto_replied' AND evidence.kind_rank <= 2
                    THEN 30 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '180 days'))
                WHEN evidence.kind = 'clicked' AND evidence.kind_rank <= 3
                    THEN 35 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '270 days'))
                WHEN evidence.kind = 'opened' AND evidence.kind_rank <= 3
                    THEN 25 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '180 days'))
                WHEN evidence.kind = 'delivered' AND evidence.kind_rank <= 4
                    THEN 14 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '180 days'))
                ELSE 0
            END
        ), 0) AS positive_score,
        COALESCE(sum(
            CASE
                WHEN evidence.kind = 'bounced_recipient' AND evidence.kind_rank <= 2
                    THEN 70 * power(0.5, GREATEST(0, extract(epoch FROM (NOW() - evidence.observed_at))) / extract(epoch FROM INTERVAL '365 days'))
                ELSE 0
            END
        ), 0) AS negative_score,
        max(evidence.observed_at) FILTER (
            WHERE evidence.kind IN ('delivered', 'opened', 'clicked', 'replied', 'auto_replied')
        ) AS last_positive_at,
        max(evidence.observed_at) FILTER (
            WHERE evidence.kind = 'bounced_recipient'
        ) AS last_negative_at
    FROM affected
    LEFT JOIN ranked_evidence evidence
      ON evidence.contact_id = affected.contact_id
    GROUP BY affected.contact_id
)
UPDATE contacts contact
SET verification_evidence_at = remaining.last_positive_at,
    verification_status = CASE
        WHEN contact.verification_source = 'manual' THEN contact.verification_status
        WHEN remaining.last_negative_at > COALESCE(remaining.last_positive_at, '-infinity'::timestamptz) THEN 'invalid'
        WHEN remaining.positive_score >= 20 THEN 'valid'
        ELSE contact.verification_status
    END,
    verification_confidence = CASE
        WHEN contact.verification_source = 'manual'
            THEN LEAST(100, 95 + floor(remaining.positive_score / 4)::integer)
        WHEN remaining.last_negative_at > COALESCE(remaining.last_positive_at, '-infinity'::timestamptz)
            THEN LEAST(100, 60 + floor(remaining.negative_score / 2)::integer)
        WHEN remaining.positive_score >= 20
            THEN LEAST(100, 70 + floor(remaining.positive_score / 2)::integer)
        ELSE contact.verification_confidence
    END,
    verification_reason = CASE
        WHEN contact.verification_source = 'manual' THEN contact.verification_reason
        WHEN remaining.last_negative_at > COALESCE(remaining.last_positive_at, '-infinity'::timestamptz)
            THEN 'recipient address bounced after the last positive mail evidence'
        WHEN remaining.positive_score >= 20
            THEN 'real mail evidence confirms the address'
        ELSE contact.verification_reason
    END,
    updated_at = NOW()
FROM remaining
WHERE contact.id = remaining.contact_id;

DROP TABLE issue_549_missed_replies;
