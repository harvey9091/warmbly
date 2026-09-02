-- The page background image is a third brand asset alongside the logo and the
-- cover. It gets its own column, like the other two, so org export carries the
-- object with the row.
ALTER TABLE forms ADD COLUMN background_url text NOT NULL DEFAULT '';
