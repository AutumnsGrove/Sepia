-- 0007_revision_target: which saved entry a working copy revises.
--
-- The target used to live only in the server's memory, so a restart mid-revision lost it: the draft was still
-- linked to its session, but confirm did not know what it revised and saved the working copy as a second entry.
-- The session is where the card belongs, so the target belongs beside sessions.entry_id.

ALTER TABLE sessions ADD COLUMN revises_entry_id TEXT REFERENCES entries (id) ON DELETE RESTRICT;
