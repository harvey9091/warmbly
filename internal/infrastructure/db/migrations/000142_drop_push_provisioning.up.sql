-- Removes the push half of fleet management.
--
-- Servers used to be bought through a cloud API from a stored template, then
-- configured over SSH from a keypair the backend held. A node now joins by
-- running one command on a machine you already have, and keeps itself current
-- by asking the control plane what version it should be. None of the pieces
-- below have a job left:
--
--   provisioning_templates/_jobs/_policy  bought and tracked cloud servers
--   aws_credentials, worker_profiles      templated env for machines we configured
--   workers.ssh_*, install_state          the SSH channel and its lifecycle
--   workers.enrollment_token_*            per-worker tokens, replaced by the
--                                         instance join token in admin_settings
--
-- decision_log stays: it is the audit trail of what the control loops decided,
-- which still matters.

BEGIN;

ALTER TABLE workers
    DROP COLUMN IF EXISTS ssh_host,
    DROP COLUMN IF EXISTS ssh_port,
    DROP COLUMN IF EXISTS ssh_user,
    DROP COLUMN IF EXISTS ssh_public_key,
    DROP COLUMN IF EXISTS ssh_private_key_encrypted,
    DROP COLUMN IF EXISTS ssh_host_fingerprint,
    DROP COLUMN IF EXISTS install_state,
    DROP COLUMN IF EXISTS enrollment_token_hash,
    DROP COLUMN IF EXISTS enrollment_token_expires_at,
    DROP COLUMN IF EXISTS profile_id,
    DROP COLUMN IF EXISTS config_applied_at;

DROP TABLE IF EXISTS provisioning_jobs;
DROP TABLE IF EXISTS provisioning_templates;
DROP TABLE IF EXISTS provisioning_policy;
DROP TABLE IF EXISTS worker_profiles;
DROP TABLE IF EXISTS aws_credentials;

DROP TYPE IF EXISTS worker_install_state;

COMMIT;
