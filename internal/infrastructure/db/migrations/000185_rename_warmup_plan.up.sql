-- The pool plan was seeded as "Self-hosted pool", and that name reached the
-- dashboard, where it read as a plan only a self-hosted instance could use. It
-- is the workspace's warmup plan: unlimited mailboxes in the premium pool,
-- for mailboxes connected on the cloud and through linked instances alike.
UPDATE plans
SET name = 'Warmup'
WHERE id = '00000000-0000-0000-0000-000000000002'
  AND name = 'Self-hosted pool';
