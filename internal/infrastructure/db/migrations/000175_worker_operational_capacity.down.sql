BEGIN;

DROP MATERIALIZED VIEW IF EXISTS worker_capacity_view;

-- Restore the provider and warmup weights used before this migration.
UPDATE workers w
   SET load_score = COALESCE((
       SELECT sum(CASE
                    WHEN ea.warmup IS NOT NULL THEN 0.4
                    WHEN ea.provider::text IN ('gmail', 'outlook') THEN 0.05
                    ELSE 1.0
                  END)
         FROM email_accounts ea
        WHERE ea.worker_id = w.id
   ), 0);

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
    n.region,
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
   FROM ((public.workers w
     JOIN public.fleet_nodes n ON ((n.id = w.id)))
     LEFT JOIN aggregated a ON ((a.worker_id = w.id)))
  WHERE n.active
  WITH NO DATA;

CREATE UNIQUE INDEX worker_capacity_view_pk ON public.worker_capacity_view USING btree (worker_id);

REFRESH MATERIALIZED VIEW public.worker_capacity_view;

COMMIT;
