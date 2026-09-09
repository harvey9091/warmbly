-- The fleet becomes pull-based, and every process that runs on a machine you
-- own becomes a node.
--
-- A node enrols with a join token, heartbeats, reports what version it is
-- running and what it is using, and asks the control plane what version it
-- SHOULD be running. Nothing reaches into a node. That replaces the push
-- model, where the backend held SSH keys, bought servers through a cloud API,
-- and shelled in to install, restart and upgrade them.
--
-- fleet_nodes is the registry every role shares. `workers` becomes a pure
-- placement extension: the columns that describe a *machine* (name, address,
-- region, version, liveness) move to the node, and the columns that describe
-- *what mail it carries* stay. workers.id IS the node id, enforced by the
-- foreign key, so "every worker is a node" cannot drift.

BEGIN;

CREATE TABLE fleet_nodes (
    id   uuid PRIMARY KEY,
    role text NOT NULL,
    name text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',

    -- Where it is and how to reach it. region is the placement hint; address
    -- is whatever the node reports as its outbound address.
    region  text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',

    -- version is what the node reports it is running. desired_version is
    -- resolved per role by the control plane; pinned_version overrides it for
    -- one node, so a single machine can be held back or canaried.
    version        text NOT NULL DEFAULT '',
    pinned_version text NOT NULL DEFAULT '',

    active       boolean NOT NULL DEFAULT true,
    last_seen_at timestamp with time zone,
    enrolled_at  timestamp with time zone NOT NULL DEFAULT now(),

    -- Usage, overwritten on every beat. Deliberately a snapshot and not a
    -- history: worker_health_samples already keeps the time series capacity
    -- math needs, and a per-node metrics table would grow without a reader.
    cpu_percent    numeric(5,2),
    memory_mb      integer,
    goroutines     integer,
    uptime_seconds bigint,

    last_error text NOT NULL DEFAULT '',

    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),

    CONSTRAINT fleet_nodes_role_check CHECK (role = ANY (ARRAY['worker'::text, 'consumer'::text]))
);

CREATE INDEX idx_fleet_nodes_role_seen ON fleet_nodes (role, last_seen_at DESC);
CREATE INDEX idx_fleet_nodes_live ON fleet_nodes (last_seen_at) WHERE active;

-- Existing workers become nodes, carrying over what they already reported.
INSERT INTO fleet_nodes (id, role, name, notes, region, address, version, active,
                         last_seen_at, enrolled_at, last_error, created_at, updated_at)
SELECT w.id, 'worker', w.name, COALESCE(w.notes, ''), COALESCE(w.region, ''),
       COALESCE(w.ip_addr, ''), COALESCE(w.image_version, ''), COALESCE(w.active, false),
       w.last_seen_at, w.created_at, COALESCE(w.last_error, ''), w.created_at, w.updated_at
  FROM workers w
ON CONFLICT (id) DO NOTHING;

-- The capacity view reads the machine's region through the node now.
DROP MATERIALIZED VIEW IF EXISTS worker_capacity_view;

ALTER TABLE workers
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS ip_addr,
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS image_version,
    DROP COLUMN IF EXISTS last_error;

ALTER TABLE workers
    ADD CONSTRAINT workers_id_is_a_node
    FOREIGN KEY (id) REFERENCES fleet_nodes(id) ON DELETE CASCADE;

-- Tags describe a machine, so they belong to the node. Repointing the foreign
-- key lets a consumer carry them too; the table keeps its name because
-- renaming it buys nothing a comment does not.
ALTER TABLE worker_tags DROP CONSTRAINT IF EXISTS worker_tags_worker_id_fkey;
ALTER TABLE worker_tags
    ADD CONSTRAINT worker_tags_node_id_fkey
    FOREIGN KEY (worker_id) REFERENCES fleet_nodes(id) ON DELETE CASCADE;

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
