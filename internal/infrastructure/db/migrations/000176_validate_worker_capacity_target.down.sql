BEGIN;

ALTER TABLE fleet_nodes
    DROP CONSTRAINT IF EXISTS fleet_nodes_capacity_target_positive,
    ADD CONSTRAINT fleet_nodes_capacity_target_positive
        CHECK (capacity_target > 0) NOT VALID;

COMMIT;
