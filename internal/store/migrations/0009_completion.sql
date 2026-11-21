-- 0009_completion: the shelf a review belongs to, and how far she got on a work she stopped.
--
-- Until now an entry was always a finished thing. That was wrong in two directions: a book she put down had to be
-- logged as if she had finished it, and a book she means to read could not be written down at all
-- (issue #30, docs/slices/S21-wishlist-and-unfinished.md).
--
-- completion is a second dimension, kept apart from status on purpose. status is the lifecycle of the card
-- (draft, then saved, and draft never comes back). completion is what she has to say about the work: a wish, a
-- review, or a work she stopped. A saved entry can be any of the three, which is exactly why one column could not
-- do both jobs.
--
-- Every row that exists is 'finished', which is what the app believed about all of them until now. The default does
-- that in place, so a deployment that is already running keeps every entry and its library page unchanged.
--
-- progress is "how far I got", one short line in her own words, and it is only ever written for an 'abandoned'
-- entry. It is bounded in the domain (200 characters) and in the API, not here, because a SQLite CHECK on a length
-- would fail a migration on an existing row rather than explain itself to her.

ALTER TABLE entries ADD COLUMN completion TEXT NOT NULL DEFAULT 'finished'
    CHECK (completion IN ('want_to', 'finished', 'abandoned'));

ALTER TABLE entries ADD COLUMN progress TEXT NOT NULL DEFAULT '';

-- The library's newest order, sliced by shelf. The existing entries_library index is (status, deleted_at,
-- finished_on, id); putting completion after the two filters lets one index serve both the shelf chip and the
-- default library shelf, which is "everything she has not only put on a list".
CREATE INDEX entries_library_shelf
    ON entries (status, deleted_at, completion, finished_on DESC, id DESC);
