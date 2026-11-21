# S21. Wishlist and didn't finish

## Purpose

Autumn starts things and stops, and she also wants to keep a list of things she means to start. Today the
library only knows how to hold a finished review, so both cases have no honest home: a dropped book gets
logged as if it were finished, and a book she intends to read cannot be written down at all.

This slice gives an entry a **completion** state with three shelves — `want_to`, `finished` and `abandoned` —
and the plain words and one badge that let her see which one a row is. It is the smallest change that makes
the app honest about a stopped book, so that issue [#31](https://github.com/AutumnsGrove/Sepia/issues/31)
can later design the reading screens properly around the three.

## Depends on

S04 (the entries table and its queries), S09 (the REST and socket contract), S12 (the library, the card and
the reading state). All three are done, so this can start immediately and needs no other slice.

**S13 and S14 already run against this schema**, so the migration must leave a running deployment readable:
existing rows become `finished`, and nothing else changes.

## Where we start

[`../../PLAN.md`](../../PLAN.md) sections 6 (the schema) and 11 (the rules). [`../ENTRY_CARD.md`](../ENTRY_CARD.md)
sections 1 and 2. Issue
[#30](https://github.com/AutumnsGrove/Sepia/issues/30), which proposed the state and asked the questions this
slice settles. Mockup page 03 for the library row and page 02 for the card.

## Settled

Everything here was answered by Autumn on 2026-11-20, in the batch before the slice started. The answers are
recorded as decisions below; nothing in this section is an assumption.

- An entry has a `completion` of `want_to`, `finished` or `abandoned`. It is a second dimension, kept apart
  from `status` (`draft | saved`), because one is the lifecycle of a card and the other is what she has to say
  about the work.
- A finished entry still needs a rating and a body. An abandoned entry needs a body and no rating. A wanted
  entry needs neither, only the work and her confirm tap.
- `finished_on` keeps its column and its name in the database and the API. The card relabels it: "Finished" for
  a finished entry, "Stopped" for an abandoned one. There is no second date column.
- An abandoned entry may carry one optional line of **how far she got**, in her own words.
- The badge reads **didn't finish**. A wanted row reads **wanted**. A finished row has no badge, because it is
  what the library already was.
- The library shows finished and abandoned together, and a wanted row is behind one extra chip.
- Logging a review for a work on the wanted list edits that row into the entry. It does not create a second row.

## Scope

Each stage is one sitting and leaves `make check` passing.

1. **S21.1 Domain.** `domain.Completion` with its three values and a `CompletionState()` that reads an empty
   value as `finished`, so every existing `domain.Entry` literal and stored row stays valid. `Entry` gains
   `Completion` and `Progress`. `ValidateForSave` relaxes to: body always required except for a wish, rating
   required only when finished, `progress` bounded and containment-checked like the verdict. `Diff` gains both
   fields.
2. **S21.2 Deterministic cues.** `domain.CompletionCues(text)` reads the phrases a person actually writes
   ("couldn't get into it", "gave up on it", "dnf", "didn't finish", "want to read", "on my list") into one
   completion value. Plain rules, no model, so the same words always give the same answer. No new decisions
   check: the deterministic parse is where `PLAN.md` 5 step 3 puts a field like this, and a new check would
   need threshold tuning, which is gate G4.
3. **S21.3 Data layer.** Migration `0009_completion.sql` adds `completion TEXT NOT NULL DEFAULT 'finished'`
   and `progress TEXT NOT NULL DEFAULT ''`, plus the index the new filter needs. The library queries gain a
   `completion` filter and the rating order puts a finished row above an unfinished one before it compares
   ratings. The cursor for that order grows one key, and the old three-key cursor is refused by the existing
   signature check rather than read wrongly.
4. **S21.4 Session engine.** The draft patch accepts `completion` and `progress`. `waitingReason` stops asking
   an abandoned card for a rating and stops asking a wanted card for anything but its work.
5. **S21.5 Agent.** `applyParsed` fills `completion` from the cues on a fresh card. The prompts say what each
   shelf needs and that a "how far" line is quoted from her, never written. `save_entry` accepts `completion`
   and `progress`, with the same quote check the verdict gets.
6. **S21.6 Contract.** The `Entry` schema gains `completion` (required) and `progress` (optional, 200
   characters), `rating_q`'s minimum becomes 0 because a wish and an abandoned entry have no rating, and
   `GET /api/entries` gains a `completion` filter. `api/error-codes.json` gains the new guard codes. The draft
   snapshot golden example gains the two fields.
7. **S21.7 Frontend.** The card shows the shelf, the relabelled date and the optional how-far line. The library
   row shows a quiet badge. The library gains one chip for the wanted shelf, and its month grouping is skipped
   there because "the month I finished it" means nothing for a book I have not read.
8. **S21.8 The wanted row becomes a review.** `save_entry` on a work that has a saved wanted row opens a
   revision of that row rather than a new entry, so the wish is the start of the entry rather than a duplicate.

## Tests and exit

Named before the code, in the order they are written.

| Route | Cases |
|---|---|
| Happy | An abandoned message ("couldn't get into it, about 100 pages in") builds a card with no rating, saves, and shows `didn't finish` | 
| Happy | A wanted message ("want to read Piranesi") builds a card with no rating and no body, and saves on the confirm tap |
| Happy | A finished entry is unchanged: rating required, badge absent, byte-identical golden examples apart from the new fields |
| Bad | An abandoned card with no body is refused with `body_required`; a wanted card is not |
| Bad | A progress line the model wrote but she did not is refused with `progress_not_your_words` |
| Bad | A completion value outside the three is refused by the domain and by the database CHECK |
| Bad | The old three-key rating cursor is refused rather than read as a four-key one |
| Bad | An 8,001-character progress line is refused with `progress_too_long` |
| Sad | A migration from a database with saved entries leaves every row `finished` and the library page identical |
| Sad | A wanted row with no date sorts into the undated tail of the newest order without breaking the cursor |
| Sad | The decisions backend being off changes nothing about the completion read, because it is code |
| Fuzz | `CompletionCues` over arbitrary bytes: always one of the three values, never a panic |

## Gates

None. No live provider, no new dependency, no image leaves the device. The one rule change (a rating is no
longer required in every case) is decided above and written into `PLAN.md` in the same change, so it is not
trigger T10.

## Open questions

None outstanding. The issue's four were answered before the slice started; the answers are below.

## Decisions

Answered by Autumn on 2026-11-20, in the batch before the slice started.

1. **Three states, on one table.** `want_to`, `finished`, `abandoned` as a `completion` column on `entries`,
   not a separate wanted table. A wish inherits soft delete, revisions, the answers and the whole list plumbing
   that already exists, and "edit the wish into the entry" needs the row to be the same row.
2. **The rating rule relaxes, and it is written down.** A finished entry still needs a rating and a body. An
   abandoned entry needs a body. A wanted entry needs neither. `PLAN.md` 11 rule 6 and `ENTRY_CARD.md` 2 are
   updated in the same commit, because a spec that says otherwise would be trigger T11.
3. **One date column, two labels.** `finished_on` keeps its name in the schema and the API and gains no sibling.
   The card reads "Finished" or "Stopped" from the completion state. Renaming the column was rejected as churn
   across every query, the openapi schema and the goldens for a word the UI already handles.
4. **The badge says "didn't finish", the wanted row says "wanted".** Plain, states the fact, does not judge it.
   A finished row carries no badge at all.
5. **Abandoned rows live in the library, out of the rating order.** They appear in the newest order like any
   other row, with their badge. Under "Highest rated" the finished rows come first and the unfinished tail
   follows, because an unrated row has no rank to take. The wanted shelf is one more chip beside the medium
   chips and shows a flat list rather than month groups.
6. **A "how far" line is her words.** One optional line, 200 characters, extracted or typed exactly like the
   body, containment-checked when extracted. It is stored in `progress` and shown only for an abandoned entry.
   A structured page or episode count was rejected: it needs a unit per medium and it cannot hold "two
   episodes and the first ten minutes of the third".
7. **The cues are code, not a decisions check.** `PLAN.md` 5 puts a deterministic field before the decision
   model, and a new check would need thresholds tuned on real messages, which is gate G4. Abandoned wins when
   one message carries both ("I want to read it but I couldn't get into it"), because the stronger statement
   about the thing is the one she is reporting.
8. **The wanted row is revised, not duplicated.** Logging a review for a work whose only saved row is a wish
   opens a revision of that row. It keeps its id, its history and its place in the library.
9. **Two calls the spec did not settle, made and written down.** The undated tail of the newest order is still
   ordered by id rather than by when she added the row, because changing that order means a third cursor key and
   that is a list decision, not a shelf one. And the shelf chips are plain chips beside the medium chips rather
   than a designed set of shelves. Both belong to issue #31, the library design refresh, and are recorded in
   [`../BUILD-LOG.md`](../BUILD-LOG.md) so the next agent does not rediscover them.

## Done when

- The three shelves save through the real socket, with the card, the badge and the chip showing which one a row
  is, checked by eye at phone width.
- `make check` is green, including the race tests, the frontend checks and the guard.
- `PLAN.md`, `docs/ENTRY_CARD.md`, the slice table, `api/openapi.yaml`, `api/error-codes.json` and the golden
  examples all agree with the code in the same commit.
- A line is in [`../BUILD-LOG.md`](../BUILD-LOG.md).
