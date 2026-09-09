-- Restores the four worker categories. The original per-worker values are not
-- recoverable, so every worker comes back as a shared, premium, clean,
-- cold_smtp box, which is the default a fresh install would have produced.

BEGIN;

DROP MATERIALIZED VIEW IF EXISTS worker_capacity_view;

DROP INDEX IF EXISTS idx_email_accounts_worker_assigned_at;

ALTER TABLE email_accounts DROP COLUMN IF EXISTS worker_assigned_at;

ALTER TABLE workers DROP COLUMN IF EXISTS region;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'worker_risk_pool') THEN
        CREATE TYPE public.worker_risk_pool AS ENUM ('clean', 'risky', 'quarantine');
    END IF;
END
$$;

ALTER TABLE workers
    ADD COLUMN IF NOT EXISTS free_tier boolean DEFAULT false NOT NULL,
    ADD COLUMN IF NOT EXISTS worker_type text DEFAULT 'shared'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS risk_pool public.worker_risk_pool DEFAULT 'clean'::public.worker_risk_pool NOT NULL,
    ADD COLUMN IF NOT EXISTS egress_kind text DEFAULT 'cold_smtp'::text NOT NULL;

ALTER TABLE workers
    ADD CONSTRAINT workers_egress_kind_check
    CHECK ((egress_kind = ANY (ARRAY['cold_smtp'::text, 'oauth_api'::text, 'warmup_only'::text])));

ALTER TABLE provisioning_templates
    ADD COLUMN IF NOT EXISTS tier text DEFAULT 'shared_premium'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS egress_kind text DEFAULT 'cold_smtp'::text NOT NULL;

ALTER TABLE provisioning_templates
    ADD CONSTRAINT provisioning_templates_tier_check
    CHECK ((tier = ANY (ARRAY['shared_free'::text, 'shared_premium'::text, 'dedicated'::text])));

ALTER TABLE provisioning_templates
    ADD CONSTRAINT provisioning_templates_egress_kind_check
    CHECK ((egress_kind = ANY (ARRAY['cold_smtp'::text, 'oauth_api'::text, 'warmup_only'::text])));

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
    w.worker_type,
    w.free_tier,
    w.egress_kind,
    w.health_state,
    w.load_score,
    (
        CASE w.egress_kind
            WHEN 'cold_smtp'::text THEN 16
            WHEN 'oauth_api'::text THEN 400
            WHEN 'warmup_only'::text THEN 25
            ELSE 16
        END)::numeric AS base_capacity,
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
