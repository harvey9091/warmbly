-- One kind of worker.
--
-- Workers used to carry four operator- or plan-chosen categories (free_tier,
-- worker_type, risk_pool, egress_kind) and placement was a filter over them.
-- None of the four survives contact with how this product actually sends:
-- a worker never talks to a recipient MX, it authenticates to the customer's
-- own mailbox provider, which then does the delivery from its own outbound
-- pool. The worker's IP is therefore invisible to recipient spam filtering
-- (Google strips the submitting client IP) and matters only to the provider,
-- as a login-trust and connection-concurrency surface.
--
-- So co-locating a "risky" mailbox next to a clean one cannot contaminate the
-- clean one's sending reputation, and segregating free from paid buys nothing
-- a health signal doesn't already buy. Placement becomes a score over live
-- health, load, affinity and blast radius; the categories go.

BEGIN;

DROP MATERIALIZED VIEW IF EXISTS worker_capacity_view;

ALTER TABLE workers
    DROP COLUMN IF EXISTS free_tier,
    DROP COLUMN IF EXISTS worker_type,
    DROP COLUMN IF EXISTS risk_pool,
    DROP COLUMN IF EXISTS egress_kind;

DROP TYPE IF EXISTS worker_risk_pool;

-- Region is an affinity hint, never a partition. A mailbox scores better on a
-- worker whose egress geolocates near where its provider expects the account
-- to sign in from; an empty region simply scores neutral.
ALTER TABLE workers ADD COLUMN IF NOT EXISTS region text NOT NULL DEFAULT '';

-- Residency bookkeeping. Moving a mailbox changes the client IP its provider
-- sees, which is a trust event worth avoiding, so the placement loop enforces
-- a minimum residency and a per-mailbox cooldown. The old rebalancer
-- documented a 24h cooldown but had no column to enforce it with.
ALTER TABLE email_accounts
    ADD COLUMN IF NOT EXISTS worker_assigned_at timestamptz;

UPDATE email_accounts
   SET worker_assigned_at = now()
 WHERE worker_id IS NOT NULL
   AND worker_assigned_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_email_accounts_worker_assigned_at
    ON email_accounts (worker_assigned_at)
    WHERE worker_id IS NOT NULL;

-- Provisioning templates categorised the machines they created. They now
-- describe only the machine shape; what lands on it is the placer's call.
ALTER TABLE provisioning_templates
    DROP CONSTRAINT IF EXISTS provisioning_templates_tier_check,
    DROP CONSTRAINT IF EXISTS provisioning_templates_egress_kind_check;

ALTER TABLE provisioning_templates
    DROP COLUMN IF EXISTS tier,
    DROP COLUMN IF EXISTS egress_kind;

-- base_capacity is now one number for every worker, in cold-mailbox
-- equivalents. It does not need to branch on an egress category because each
-- mailbox already contributes its own weight (see worker.MailboxWeight): an
-- OAuth-API mailbox costs 0.05 and a cold SMTP mailbox costs 1.0, so a worker
-- carrying either kind converges on the same load number without the worker
-- having to declare which kind it expects.
CREATE MATERIALIZED VIEW worker_capacity_view AS
 WITH aggregated AS (
         SELECT worker_health_samples.worker_id,
            sum(worker_health_samples.sends_attempted) AS sends_attempted_1h,
            sum(worker_health_samples.sends_succeeded) AS sends_succeeded_1h,
            sum(worker_health_samples.bounces_hard) AS bounces_hard_1h,
            sum(worker_health_samples.bounces_soft) AS bounces_soft_1h,
            sum(worker_health_samples.complaints) AS complaints_1h,
            sum(worker_health_samples.auth_errors) AS auth_errors_1h
           FROM public.worker_health_samples
          WHERE (worker_health_samples.observed_at > (now() - '01:00:00'::interval))
          GROUP BY worker_health_samples.worker_id
        )
 SELECT w.id AS worker_id,
    w.region,
    w.health_state,
    w.load_score,
    (16)::numeric AS base_capacity,
    GREATEST(0.0, LEAST(1.0, ((1.0 - LEAST(0.5, (((COALESCE(a.bounces_hard_1h, (0)::bigint))::numeric / (NULLIF(a.sends_attempted_1h, 0))::numeric) * (5)::numeric))) - LEAST(0.5, (((COALESCE(a.complaints_1h, (0)::bigint))::numeric / (NULLIF(a.sends_attempted_1h, 0))::numeric) * (100)::numeric))))) AS health_multiplier,
    LEAST(1.0, (EXTRACT(epoch FROM (now() - w.created_at)) / ((72 * 3600))::numeric)) AS age_multiplier,
    COALESCE(a.sends_attempted_1h, (0)::bigint) AS sends_attempted_1h,
    COALESCE(a.sends_succeeded_1h, (0)::bigint) AS sends_succeeded_1h,
    COALESCE(a.bounces_hard_1h, (0)::bigint) AS bounces_hard_1h,
    COALESCE(a.bounces_soft_1h, (0)::bigint) AS bounces_soft_1h,
    COALESCE(a.complaints_1h, (0)::bigint) AS complaints_1h,
    COALESCE(a.auth_errors_1h, (0)::bigint) AS auth_errors_1h
   FROM (public.workers w
     LEFT JOIN aggregated a ON ((a.worker_id = w.id)))
  WHERE w.active
  WITH NO DATA;

CREATE UNIQUE INDEX worker_capacity_view_pk ON public.worker_capacity_view USING btree (worker_id);

REFRESH MATERIALIZED VIEW public.worker_capacity_view;

COMMIT;
