-- Keep the tracked-send join fast without locking email_tasks writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_tasks_tracked
    ON email_tasks (task_id)
    WHERE tracked;
