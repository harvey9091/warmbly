BEGIN;

ALTER TABLE fleet_nodes
    DROP CONSTRAINT IF EXISTS fleet_nodes_capacity_target_positive,
    DROP COLUMN IF EXISTS capacity_target;

COMMIT;
