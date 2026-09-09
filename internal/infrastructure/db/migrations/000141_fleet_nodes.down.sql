-- Puts the machine columns back on workers and drops the node registry.
-- Consumer nodes have nowhere to go in the old shape, so they are lost.

BEGIN;

DROP MATERIALIZED VIEW IF EXISTS worker_capacity_view;

ALTER TABLE workers DROP CONSTRAINT IF EXISTS workers_id_is_a_node;

ALTER TABLE workers
    ADD COLUMN IF NOT EXISTS name character varying(255) DEFAULT ''::character varying NOT NULL,
    ADD COLUMN IF NOT EXISTS notes text,
    ADD COLUMN IF NOT EXISTS ip_addr text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS region text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS active boolean DEFAULT false,
    ADD COLUMN IF NOT EXISTS last_seen_at timestamp with time zone,
    ADD COLUMN IF NOT EXISTS image_version text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS last_error text;

UPDATE workers w
   SET name          = n.name,
       notes         = NULLIF(n.notes, ''),
       ip_addr       = n.address,
       region        = n.region,
       active        = n.active,
       last_seen_at  = n.last_seen_at,
       image_version = n.version,
       last_error    = NULLIF(n.last_error, '')
  FROM fleet_nodes n
 WHERE n.id = w.id;

-- Drop tag rows that belong to non-worker nodes before the key points back at
-- workers, or the constraint cannot be created.
DELETE FROM worker_tags t WHERE NOT EXISTS (SELECT 1 FROM workers w WHERE w.id = t.worker_id);

ALTER TABLE worker_tags DROP CONSTRAINT IF EXISTS worker_tags_node_id_fkey;
ALTER TABLE worker_tags
    ADD CONSTRAINT worker_tags_worker_id_fkey
    FOREIGN KEY (worker_id) REFERENCES workers(id) ON DELETE CASCADE;

DROP TABLE IF EXISTS fleet_nodes;

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
