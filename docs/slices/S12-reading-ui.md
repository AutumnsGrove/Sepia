# S12. Reading UI

## Purpose

The calm half of the app: everything you look at after the review is written. Home, Drafts, the Library, a
saved entry with its history, and deletion that never erases. Plain rows, no card chrome, quick to scan on a
phone.

## Depends on

S10 and S04's queries through the S09 REST endpoints (or the dev stack). **S13 depends on it** for the Settings
shell.

## Where we start

Mockup pages 03 and 04 and the empty states on page 07; [`../../PLAN.md`](../../PLAN.md) section 2 (Home,
Drafts, Tags).

## Settled

- Home is a launcher: drafts that are waiting, recent entries, a first-run empty state with tap-to-fill
  examples, the composer where the thumb is, and Drafts, Library and Settings in the top bar with a drafts
  badge.
- The Drafts list states what each draft is waiting for; drafts never expire.
- The Library is grouped by month with medium chips, a sort, and cursor pagination; the entry page shows the
  latest entry, the other entries on the same work, details, and earlier versions with Restore as draft.
- Delete exists only in the entry menu, confirms inline with Keep as the default, goes to a Deleted list with
  Restore, and is never offered in chat.
- Search and the Ask button arrive in S16; their places are reserved.

## Scope

1. **S12.1 Home.** The launcher, the first-run state, the composer routing a first message to a new entry
   session, and the badge.
2. **S12.2 Drafts.** The list with waiting reasons and resume.
3. **S12.3 Library.** Groups, chips, sort, pagination, the empty and no-results states.
4. **S12.4 Entry page.** The saved view, other entries on the work, details, the versions screen, restore as a
   draft revision, Edit and Log again.
5. **S12.5 Delete and Deleted.** The menu, the inline confirmation, the Deleted list, Restore.
6. **S12.6 Settings shell.** The settings list and its rows (Updates and Services arrive in S13).

## Tests and exit

| Route | Cases |
|---|---|
| Happy | Each mockup frame on pages 03 and 04 (except chat) has a state, a baseline and an axe pass; resuming a draft returns to its card |
| Bad | A cursor from another query is rejected; a huge title and a long review render without breaking layout; delete is not reachable from chat; a malformed entry id shows a not-found state |
| Sad | The library loading slowly shows skeleton rows, not a spinner in the middle; a failed list shows a retry; a deleted entry reopened by a stale link says so |

## Gates

None.

## Open questions

None. The one open question was answered before the slice started, and is recorded below.

## Decisions

Answered by Autumn on 2026-11-10, in the batch before the slice started. Each answer is a decision the code and
the contract now carry; anything not listed here was settled already.

1. **Page size and skeleton rows: 50 and 8.** 50 is already the list cap in [`STANDARDS.md`](../STANDARDS.md) 4a,
   so the default page and the documented limit agree. Eight skeleton rows fill one phone screen without
   pretending to know how many rows are coming.
2. **The contract gains five additions**, each following from text already settled here or in `PLAN.md`: a `sort`
   on `GET /api/entries` (`newest`, the default, and `rating`); a `work` filter, for the other entries on one work;
   a `deleted` filter plus `POST /api/entries/{id}/restore`, for the Deleted list; and two new fields on the
   socket's `reopen`, `mode` (`revision` by default, `new` for Log again) and `revision` (an integer, for Restore
   as draft). A seventh and eighth follow from decision 3: `GET /api/settings` and `PUT /api/settings`.
3. **Settings shell: Deleted entries and a working Quick pre-read switch.** Updates and Services arrive with S13,
   Backups and export with S14 and S19, and neither is shown now as a row that does nothing: a disabled row is a
   lie in the same way an empty screen that says nothing is. The switch is stored in the database (migration 0008,
   one settings row), read on each turn. Off means no decisions call **and no notice**, because it was her
   choice; "not configured" keeps its notice, because that one is not.
4. **The two counts read a page, not a total.** The drafts badge and the Deleted count each ask for one page at the
   documented maximum (100) and show its length, or `99+` above it. No count field is added to the list envelope:
   a second query on every list to save one request on two screens is not a trade worth making for one person.
5. **The Deleted row shows the date and nothing else.** The mockup's "· a duplicate" is prose the schema has no
   field for and the inline confirm has no box to type it in, so it is dropped rather than invented.
6. **Two sorts, both total.** `newest` is `finished_on` descending then `id` descending; `rating` is `rating_q`
   descending, then `finished_on`, then `id`. A saved entry has every one of those keys, so a page can never
   repeat or skip a row.
7. **A cursor belongs to its query.** It carries a short hash of the filters and the sort it was made under, and a
   cursor from another query is refused with `invalid_value`, which is the bad route below.
8. **A deleted entry read by a stale link answers 200**, with `deleted_at` set, and the page says it was deleted
   and offers Restore. Answering 404 would leave her without the way back, which is the point of soft delete.
9. **Three mockup details are deliberately not built.** The snippet under a draft's row needs the draft's last
   message, which the session list does not carry; adding a second copy of her words to a list response for
   decoration is not a trade worth making. The dash icon beside a waiting reason and the delete reason on a Deleted
   row ("· a duplicate") are the same: nothing stores them, and a control or a field that invents what it shows is
   worse than one that is absent. The rest of the frames on pages 03, 04 and 07 are built as drawn.
10. **Log again and Restore as draft are both `reopen`.** Log again sends `mode: "new"`: a fresh draft on the same
   work with no body and no rating, so the card asks for them. Restore as draft sends `revision: n`: a working
   copy of that version's snapshot, so confirming makes it the newest version and keeps the one it replaced.

## Done when

The reading screens match the mockups on a phone, every empty and error state has a test, and soft delete and
restore are demonstrated through the UI.

**Status: built.** Fourteen end-to-end states on each of two browsers, each with an axe pass and a baseline, against a
seeded stack of its own; the Go and Vitest suites cover the new boundary's three routes. Two live checks are left for
Autumn, in [`../BUILD-LOG.md`](../BUILD-LOG.md): the Linux baselines are made by the `baselines` workflow, and the
screens have not been looked at on the potato.
