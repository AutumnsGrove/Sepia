-- 0010_search: the library searchable by her own words (S16.1).
--
-- SQLite's FTS5 over three things, in the order they matter when they all match: the work's title, the body she
-- wrote, and the answers she gave a question card. The ranking weights live in the query, not here, so tuning them
-- is a code change with a test rather than a migration (docs/slices/S16-search-and-chat.md, Decisions 1).
--
-- The index is derived data, not history. It holds no entry that the entries table does not already hold, so rows
-- here may be deleted and rebuilt, unlike the tables the invariant-4 triggers protect. It is kept in step by
-- triggers rather than by Go, because every write that changes searchable text goes through the database and a
-- repository that forgot to update an index is exactly the bug a trigger cannot have. A rebuild command exists for
-- the case the triggers cannot cover: a file restored or edited by hand (sepia reindex).
--
-- A wanted work is indexed too. "Have I already got this?" is the question search most often answers, and a work
-- she means to read is on her list even though it has no finish date to group by.
--
-- entry_id is UNINDEXED: it is there to join back to the entry, and it must not be searchable text.

CREATE VIRTUAL TABLE entries_fts USING fts5(
    entry_id UNINDEXED,
    title,
    body,
    answers,
    tokenize = 'unicode61 remove_diacritics 2'
);

-- The index text for one entry, recomputed from the rows themselves. Every trigger below writes the same row, so
-- there is one definition of what searchable text is and no path can disagree with another about it.
--
-- The answers are flattened from the stored JSON arrays, joined with a space, because a chip is a word like any
-- other. A prompt is the app's question, not her answer, so it is not indexed.
CREATE TRIGGER entries_fts_insert AFTER INSERT ON entries
WHEN NEW.status = 'saved' AND NEW.deleted_at IS NULL
BEGIN
    INSERT INTO entries_fts (entry_id, title, body, answers)
    SELECT e.id, w.title, e.body,
           COALESCE((SELECT group_concat(j.value, ' ')
                     FROM answers a, json_each(a.answer_json) j
                     WHERE a.entry_id = e.id), '')
    FROM entries e JOIN works w ON w.id = e.work_id
    WHERE e.id = NEW.id;
END;

-- A card confirmed, an entry edited, a review soft-deleted and a deleted one restored all land here. The delete is
-- unconditional so a row that should no longer be indexed is removed rather than left behind.
CREATE TRIGGER entries_fts_update AFTER UPDATE OF status, deleted_at, body ON entries
BEGIN
    DELETE FROM entries_fts WHERE entry_id = OLD.id;
    INSERT INTO entries_fts (entry_id, title, body, answers)
    SELECT e.id, w.title, e.body,
           COALESCE((SELECT group_concat(j.value, ' ')
                     FROM answers a, json_each(a.answer_json) j
                     WHERE a.entry_id = e.id), '')
    FROM entries e JOIN works w ON w.id = e.work_id
    WHERE e.id = NEW.id AND e.status = 'saved' AND e.deleted_at IS NULL;
END;

-- An answer is written once (answers are append-only), and it can arrive before the card is confirmed. The entry's
-- whole row is rewritten, so an answer added to a draft is already indexed when the confirm lands.
CREATE TRIGGER entries_fts_answer AFTER INSERT ON answers
BEGIN
    DELETE FROM entries_fts WHERE entry_id = NEW.entry_id;
    INSERT INTO entries_fts (entry_id, title, body, answers)
    SELECT e.id, w.title, e.body,
           COALESCE((SELECT group_concat(j.value, ' ')
                     FROM answers a, json_each(a.answer_json) j
                     WHERE a.entry_id = e.id), '')
    FROM entries e JOIN works w ON w.id = e.work_id
    WHERE e.id = NEW.entry_id AND e.status = 'saved' AND e.deleted_at IS NULL;
END;

-- A work's title can be corrected after the review is saved (a merge, a provider fix), and search must follow the
-- title she would look for rather than the one that was there the day the entry landed.
CREATE TRIGGER entries_fts_work_title AFTER UPDATE OF title ON works
BEGIN
    DELETE FROM entries_fts WHERE entry_id IN (SELECT id FROM entries WHERE work_id = NEW.id);
    INSERT INTO entries_fts (entry_id, title, body, answers)
    SELECT e.id, w.title, e.body,
           COALESCE((SELECT group_concat(j.value, ' ')
                     FROM answers a, json_each(a.answer_json) j
                     WHERE a.entry_id = e.id), '')
    FROM entries e JOIN works w ON w.id = e.work_id
    WHERE w.id = NEW.id AND e.status = 'saved' AND e.deleted_at IS NULL;
END;
