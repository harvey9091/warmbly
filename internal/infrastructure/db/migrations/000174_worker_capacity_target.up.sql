BEGIN;

ALTER TABLE fleet_nodes
    ADD COLUMN capacity_target numeric(10,2) NOT NULL DEFAULT 100,
    ADD CONSTRAINT fleet_nodes_capacity_target_positive
        CHECK (capacity_target > 0) NOT VALID;

COMMIT;
