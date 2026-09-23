-- The hierarchy delimiter a server reported for each folder is kept with the
-- folder, so the control plane can tell a subfolder from a sibling whose
-- name merely starts the same way. The worker matches a skipped folder's
-- subfolders on it; the purge that follows a folder being excluded from sync
-- has to reach the same set of stored rows. Empty when the server reported
-- none, where a folder has no subfolders to speak of.
ALTER TABLE public.unibox_mailboxes
    ADD COLUMN delim text NOT NULL DEFAULT '';
