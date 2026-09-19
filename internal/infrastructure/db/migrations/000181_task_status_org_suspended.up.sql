-- A warmup send held for a suspended workspace has written this status since
-- the org risk posture shipped, but nothing added it, so every write failed and
-- left the task pending for the dispatcher to fire again.
ALTER TYPE public.task_status ADD VALUE IF NOT EXISTS 'skipped_org_suspended';
