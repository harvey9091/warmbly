-- Every password write stamps this, and a reset link issued before it is
-- refused. NULL is "never changed since this column existed", so links that
-- are outstanding at deploy time keep working.
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS password_changed_at timestamptz;
