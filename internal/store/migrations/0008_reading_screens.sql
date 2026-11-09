-- 0008_reading_screens: the settings row, and the two list orders the reading screens added.
--
-- The library gained a rating sort, which is rating_q then created_at then id. created_at rather than finished_on
-- is deliberate: a saved entry may have no finish date, and a page cursor needs a total order over columns that are
-- never null, or a row after the boundary is skipped (docs/slices/S12-reading-ui.md, Decisions 6).
--
-- The Deleted list is its own order, newest deletion first, and its own partial index. It is a small list on
-- purpose: deleted rows never join the library again unless she restores one.

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX entries_by_rating
    ON entries (status, deleted_at, rating_q DESC, created_at DESC, id DESC);

CREATE INDEX entries_deleted
    ON entries (status, deleted_at DESC, id DESC) WHERE deleted_at IS NOT NULL;
