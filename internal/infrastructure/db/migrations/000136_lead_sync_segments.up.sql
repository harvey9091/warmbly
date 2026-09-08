-- Segment targets for a saved Google Sheets sync source.
--
-- A file import can pin every row into a segment (ContactImportCommit.segment_ids);
-- a sheet sync committed through the same importer could not, so a sync created
-- from a segment's member list produced contacts that were nowhere in it.
-- jsonb for the same reason category_ids is: it is a list handed straight to
-- the importer, never filtered in SQL beyond a containment lookup.

ALTER TABLE lead_sync_sources
    ADD COLUMN segment_ids jsonb NOT NULL DEFAULT '[]';
