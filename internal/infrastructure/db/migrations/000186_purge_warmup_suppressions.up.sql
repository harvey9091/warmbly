-- A warmup send's bounce or complaint never suppresses a recipient; drop entries only warmup evidence explains.
DELETE FROM suppressed_recipients s
WHERE s.source IN ('bounce', 'complaint')
  AND EXISTS (
      SELECT 1
      FROM deliverability_events de
      JOIN tasks t ON t.id = de.task_id
      WHERE de.organization_id = s.organization_id
        AND lower(de.recipient_email) = s.email
        AND de.event_type IN ('bounce', 'complaint')
        AND t.task_type = 'warmup'
  )
  AND NOT EXISTS (
      SELECT 1
      FROM deliverability_events de
      LEFT JOIN tasks t ON t.id = de.task_id
      WHERE de.organization_id = s.organization_id
        AND lower(de.recipient_email) = s.email
        AND de.event_type IN ('bounce', 'complaint')
        AND (t.id IS NULL OR t.task_type <> 'warmup')
  );

-- Warmup bounces and complaints belong to warmup health, not the campaign rates.
DELETE FROM deliverability_events de
USING tasks t
WHERE t.id = de.task_id
  AND t.task_type = 'warmup'
  AND de.event_type IN ('bounce', 'complaint');
