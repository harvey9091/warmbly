-- Repair high-confidence issue #549 false replies and add durable reply-processing claims.

ALTER TABLE unibox_emails
    ADD COLUMN campaign_reply_claimed_at timestamptz,
    ADD COLUMN campaign_reply_claim_token uuid,
    ADD COLUMN campaign_reply_processed_at timestamptz;

CREATE TEMP TABLE issue_549_false_replies AS
SELECT DISTINCT
    p.campaign_id,
    p.contact_id,
    p.sequence_id,
    p.dispatch_task_id,
    p.replied_at,
    ue.id AS source_message_id,
    ue.email_id AS source_account_id
FROM campaign_contact_progress p
JOIN campaigns campaign ON campaign.id = p.campaign_id
JOIN contacts c
  ON c.id = p.contact_id
 AND c.organization_id = campaign.organization_id
JOIN tasks parent_task ON parent_task.id = p.dispatch_task_id
JOIN email_accounts source_account
  ON source_account.organization_id = campaign.organization_id
JOIN unibox_emails ue
  ON ue.email_id = source_account.id
 AND (ue.folder = 'sent' OR ue.provider_folder = 'sent')
 AND (
        p.replied_at BETWEEN ue.created_at
                         AND ue.created_at + INTERVAL '5 minutes'
        OR EXISTS (
            SELECT 1
            FROM reply_intents sender_intent
            WHERE sender_intent.campaign_id = p.campaign_id
              AND sender_intent.task_id = p.dispatch_task_id
              AND sender_intent.created_at BETWEEN p.replied_at - INTERVAL '30 seconds'
                                               AND p.replied_at + INTERVAL '30 seconds'
              AND lower(sender_intent.contact_email) IN (
                    lower(source_account.email),
                    lower(COALESCE(NULLIF(source_account.send_as_email, ''), source_account.email))
                )
        )
    )
 AND EXISTS (
        SELECT 1
        FROM unnest(ue.in_reply_to) AS refs(message_id)
        WHERE btrim(refs.message_id, '<> ') = btrim(parent_task.message_id, '<> ')
    )
 AND EXISTS (
        SELECT 1
        FROM unnest(ue.to_addr) AS recipients(address)
        WHERE lower(btrim(COALESCE(
            substring(recipients.address FROM '<([^>]*)>'),
            substring(recipients.address FROM '\(([^()]*)\)\s*$'),
            recipients.address
        ))) = lower(btrim(c.email))
    )
WHERE p.replied_at IS NOT NULL
  AND NOT EXISTS (
        SELECT 1
        FROM unibox_emails inbound
        JOIN email_accounts inbound_account
          ON inbound_account.id = inbound.email_id
         AND inbound_account.organization_id = campaign.organization_id
        WHERE inbound.folder NOT IN ('sent', 'drafts')
          AND inbound.provider_folder NOT IN ('sent', 'drafts')
          AND inbound.created_at BETWEEN p.replied_at - INTERVAL '30 seconds'
                                     AND p.replied_at + INTERVAL '30 seconds'
          AND EXISTS (
                SELECT 1
                FROM unnest(inbound.from_addr) AS senders(address)
                WHERE lower(btrim(COALESCE(
                    substring(senders.address FROM '<([^>]*)>'),
                    substring(senders.address FROM '\(([^()]*)\)\s*$'),
                    senders.address
                ))) = lower(btrim(c.email))
            )
    )
  AND NOT EXISTS (
        SELECT 1
        FROM reply_intents ri
        WHERE ri.campaign_id = p.campaign_id
          AND lower(ri.contact_email) = lower(c.email)
          AND ri.created_at BETWEEN p.replied_at - INTERVAL '30 seconds'
                                AND p.replied_at + INTERVAL '30 seconds'
    );

DELETE FROM contact_verification_evidence evidence
USING issue_549_false_replies false_reply
WHERE evidence.contact_id = false_reply.contact_id
  AND evidence.kind = 'replied'
  AND evidence.ref = false_reply.source_message_id::text;

WITH affected AS (
    SELECT DISTINCT contact_id
    FROM issue_549_false_replies
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
        ELSE 'unknown'
    END,
    verification_confidence = CASE
        WHEN contact.verification_source = 'manual'
            THEN LEAST(100, 95 + floor(remaining.positive_score / 4)::integer)
        WHEN remaining.last_negative_at > COALESCE(remaining.last_positive_at, '-infinity'::timestamptz)
            THEN LEAST(100, 60 + floor(remaining.negative_score / 2)::integer)
        WHEN remaining.positive_score >= 20
            THEN LEAST(100, 70 + floor(remaining.positive_score / 2)::integer)
        ELSE LEAST(100, floor(remaining.positive_score / 2)::integer)
    END,
    verification_reason = CASE
        WHEN contact.verification_source = 'manual' THEN contact.verification_reason
        WHEN remaining.last_negative_at > COALESCE(remaining.last_positive_at, '-infinity'::timestamptz)
            THEN 'recipient address bounced after the last positive mail evidence'
        WHEN remaining.positive_score >= 20
            THEN 'remaining mail evidence confirms the address'
        ELSE 'reply evidence corrected; verification pending'
    END,
    updated_at = NOW()
FROM remaining
WHERE contact.id = remaining.contact_id;

DELETE FROM reply_intents intent
USING issue_549_false_replies false_reply, email_accounts sending_account
WHERE intent.campaign_id = false_reply.campaign_id
  AND intent.task_id = false_reply.dispatch_task_id
  AND intent.created_at BETWEEN false_reply.replied_at - INTERVAL '30 seconds'
                            AND false_reply.replied_at + INTERVAL '30 seconds'
  AND sending_account.id = false_reply.source_account_id
  AND lower(intent.contact_email) IN (
        lower(sending_account.email),
        lower(COALESCE(NULLIF(sending_account.send_as_email, ''), sending_account.email))
    );

WITH cleared AS (
    UPDATE campaign_contact_progress progress
    SET replied_at = NULL,
        reply_class = '',
        reply_confidence = 0,
        reply_source = '',
        instant_fired = array_remove(instant_fired, 'reply')
    FROM issue_549_false_replies false_reply
    WHERE progress.campaign_id = false_reply.campaign_id
      AND progress.contact_id = false_reply.contact_id
      AND progress.sequence_id = false_reply.sequence_id
    RETURNING progress.campaign_id, progress.contact_id
)
UPDATE campaign_ab_assignments assignment
SET replied_at = NULL
WHERE EXISTS (
        SELECT 1
        FROM cleared
        WHERE cleared.campaign_id = assignment.campaign_id
          AND cleared.contact_id = assignment.contact_id
    )
  AND NOT EXISTS (
        SELECT 1
        FROM campaign_contact_progress progress
        WHERE progress.campaign_id = assignment.campaign_id
          AND progress.contact_id = assignment.contact_id
          AND progress.replied_at IS NOT NULL
    );

DROP TABLE issue_549_false_replies;
