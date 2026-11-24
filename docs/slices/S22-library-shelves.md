# S22. The Library's three shelves

## Purpose

Issue [#31](https://github.com/AutumnsGrove/Sepia/issues/31) asked for a design pass over the reading half of the
app once S21 gave an entry a `completion` of `want_to`, `finished` or `abandoned`. The design is done and lives in
[`mockups/03-home-and-library.html`](../../mockups/03-home-and-library.html) (frames 4 to 9) and
[`mockups/04-entries-and-chat.html`](../../mockups/04-entries-and-chat.html) (frames 1b to 1e). This slice builds it.

## Depends on

S21 (the `completion` column, the filter and the badge). Nothing else.

## Settled

Autumn did not answer these in advance. They are calls the issue left open, made on 2026-11-24 as the simplest
choice that fits what the app already does, and written here so they can be reopened cheaply.

- **Three shelves, as tabs, with a count on each.** Finished, Didn't finish, Wanted. Tabs rather than chips because
  a shelf is a different list with its own order and its own row, not a filter over one list; the app already has
  the underline `.tabs` component from the Describe step. The count on each tab is the shelf's total regardless of
  the medium chip, so the three counts answer "how many of each" at a glance and never change as she filters.
- **There is no combined list.** The Library used to show finished and stopped rows together. It no longer does:
  each opens on its own tab, and Finished is the default. A stopped work is a review of a different kind (no
  stars, a how-far line), and mixing it into a list ordered by month finished is what made the row shape and the
  rating sort awkward.
- **The medium chips stay, under the tabs, and apply to the shelf in view.** Podcast joins them in the mockup
  because the app already has it.
- **Sorting is stated per shelf.** Finished keeps its toggle (newest first or highest rated); every finished entry
  has a rating, so the rating order is total there and the unrated tail the S21 cursor carries stops mattering
  for this screen. Didn't finish is newest stopped first, and Wanted is recently added first. Neither has a
  toggle, because there is nothing to rank; each shows its order as a quiet fixed label instead.
- **Month groups.** Finished groups by the month finished. Didn't finish groups by the month stopped, because that
  is the date its row carries. Wanted is one flat list with no heading, since it has no month. Under the rating
  order Finished is one flat list, as today.
- **The row has no slot it cannot fill.** A finished row is unchanged. A stopped row has no right-hand slot, shows
  `Book · stopped 3 Nov`, and a second line quoting her how-far line only when she gave one. A wanted row has no
  right-hand slot and shows `Book · Tamsyn Muir`, the creator standing where the date would. A rating shows on a
  stopped row only if she gave one.
- **Search covers Finished and Didn't finish**, because both hold her words, and a stopped row names its shelf
  (`Didn't finish`) where the stars would sit. A wanted work has no words; it is still found by title and is not
  counted in "N entries mention". Nothing about search ranking changes.
- **The card's Facts block, by shelf.** Finished: Format and **Finished**. Didn't finish: Format, **Stopped**, and
  a full-width **How far** fact (the current section label above it is dropped; it said the same word twice).
  Wanted: the Rating and Your words blocks are not drawn, and the saved page shows `Added Tue 17 Nov` and one
  primary action, **Log a review**. Logging a wanted work opens its own row (S21.8), so the card shows a banner
  saying so and the shelf switch lands on Finished.
- **One word per shelf, everywhere.** The card's switch said "Want to" while the tab says "Wanted". Both are
  **Finished**, **Didn't finish**, **Wanted**, and so are the badge and the page stamp.
- **The shelf switch sits on its own row under the work header**, not inside the header's text column, where three
  pills wrap to two lines and leave "Wanted" alone.

## Scope

1. **S22.1 Counts.** The library needs the three shelf totals without three requests. `GET /api/entries` gains a
   small `shelf_counts` object (`finished`, `abandoned`, `want_to`) computed over the whole table and ignoring the
   medium filter, or a sibling `GET /api/entries/counts`. Pick the sibling endpoint if the list's response shape is
   golden-tested more widely than expected. Update `api/openapi.yaml` and the golden examples in the same change.
2. **S22.2 Shelves in the query.** The default (omitted) `completion` stops meaning "finished and abandoned". Make
   the Library always send a shelf. Keep the API's own default as it is so nothing else breaks, and say so in the
   openapi description. The wanted shelf orders by `created_at` descending then `id`, with a two-key cursor of its
   own: it is a separate query path from the rating order, so the four-key cursor is untouched. This settles the
   S21 note that the undated tail was ordered by id.
3. **S22.3 Library screen.** Tabs, per-shelf order label, per-shelf grouping through `sectionsFor`, the row without
   an empty slot, and the empty copy for each shelf. `dateLabel` and `entryMeta` learn "stopped".
4. **S22.4 Card and page.** The shelf switch moves to its own row and is renamed. Drop the duplicate "How far"
   label. The wanted page and the stopped page get the layouts in the mockups, and `ENTRY_CARD.md` 1 is updated.
5. **S22.5 Docs.** `slices/README.md`, a `BUILD-LOG.md` line, and the frame counts in `mockups/index.html`.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | Each tab loads its own shelf and its count; Finished still groups by month and sorts by rating; the Wanted list has no headings |
| Happy | A stopped row shows no stars and its how-far line; a stopped row with no how-far line has no second line |
| Bad | A rating order is never offered on Didn't finish or Wanted; the old combined default is not what the screen asks for |
| Bad | A stale cursor from one shelf is refused on another rather than read as that shelf's |
| Sad | An empty shelf shows its own sentence and the tab still reads 0; the counts failing to load leaves the tabs without a number, not without a list |
| Sad | A wanted row with a missing creator shows the medium alone |
| Visual | Each of the six frames is rendered at 390 px next to the real screen and compared by eye |

## Gates

None. No live provider, no dependency, no new rule: this is layout, vocabulary and order. The one contract
addition is the shelf counts, which the issue's "counts and empty states" asks for.

## Done when

- The Library, the card and the entry page match mockup frames 03/4-9 and 04/1b-1e at phone width.
- `make check` is green, including the contract tests and the golden examples.
- `PLAN.md` needs no change; `ENTRY_CARD.md`, `slices/README.md` and `BUILD-LOG.md` agree with the code.
