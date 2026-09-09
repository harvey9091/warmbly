-- Images a workspace uploads to place inside an email body (issue #380). The
-- bytes live in object storage under the public `email-images/` prefix, because
-- the recipient's mail client fetches them with no session of ours; the row is
-- the library the composer picks from and what the storage quota counts.
--
-- Deleting a row breaks the image in mail already sent, so the dashboard warns
-- before it does. user_id is nullable so an offboarded member's uploads stay in
-- the workspace library they belong to.
CREATE TABLE IF NOT EXISTS email_images (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL,
    filename        text NOT NULL,
    mime_type       text NOT NULL DEFAULT '',
    size            bigint NOT NULL DEFAULT 0,
    width           integer NOT NULL DEFAULT 0,
    height          integer NOT NULL DEFAULT 0,
    storage_key     text NOT NULL,
    url             text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- The one query the library runs: this workspace's images, newest first.
CREATE INDEX IF NOT EXISTS idx_email_images_org_created
    ON email_images (organization_id, created_at DESC);

COMMENT ON TABLE email_images IS
    'Workspace image library for email bodies. Bytes are public objects under email-images/; size counts against the org storage quota.';
