# G8: phase 1 review, before phase 2 starts (cleared 2026-11-22)

Written 2026-11-22 by an agent that read the tree and the live board cold, a day after PR #32 merged. Everything
below was checked against the repository, the running board or the CI log; none of it is copied from a build-log
entry, and where a doc and the code disagreed the code is what is written down.

**Status: cleared by Autumn on 2026-11-22.** The answer is the section at the end; the review below is left as it was
written when she answered it, including the two gaps it names.

## 1. Where this is

Phase 1 is built and merged. `main` is at `c3e3662` (PR #32, merged 2026-11-21 20:08Z), the tree is clean, and
nothing is half-finished. The potato is on `r160.c3e3662`, and it updated itself again today at 18:37Z to
`sha256:19a85eb…` with a result of `ok`.

- CI on `main` is green on all eight checks that ran: `go`, `nocgo`, `coverage`, `web`, `e2e` (168 tests), `guard`,
  `drift`, `codeql`. `deps` did not run, which is by design: it is `dependency-review-action`, gated to
  `pull_request`, so it reviews dependencies a PR introduces and has nothing to say about a push to `main`.
- Go coverage on `main` is **78.4% of statements** in total, and every floor `tools/covercheck` asserts is met:
  `domain` 96.3% against 95, `decisions` 92.1% against 88, `banks` 87.9% against 85, `providers` 92.0% against 85,
  `store` 86.5% against 84, `server` 83.5% against 80, `backup` 84.6%, `config` 89.6% and `world` 87.5% against 80.
  `agent` is not one of the floored packages; it measured 78.6%.
- 170 Go files (51,364 lines), 747 `Test`/`Fuzz` functions including **13 fuzz targets**, 19 Vitest files (161 tests), 6 Playwright
  specs at 168 tests over Chromium and WebKit on a phone viewport, 7 mockup pages and 130 committed pixel baselines.
- 9 migrations, 12 `CREATE TRIGGER` statements across `0001`, `0003` and `0004`, and a limits table with 43 bounds.

**Phase 0's gate was never formally closed**, and this review says so rather than quietly inheriting the debt: S01
(the spikes) is still `planned` and `docs/spikes/` does not exist. Section 5 has the detail.

## 2. What phase 1 set out to prove, and where each condition stands

`docs/slices/README.md` names three conditions for phase 1 to end. Two hold live; one does not.

| Condition | Evidence | Verdict |
|---|---|---|
| A review typed as a sentence is a **saved entry on the potato** | `GET /api/entries` on the board returns `{"items":[]}`: **no entry has ever been saved there.** The one real session on the board has been `waiting_reason: needs_rating` since 2026-11-15T20:22Z (its work and its id are deliberately not written here: the repo is public) | **Not demonstrated.** The path is built and heavily tested against `internal/world`; the last mile has never run on the real box |
| The app **updates itself** | `/api/update/status` on the board: an update finished 2026-11-22T18:37:40Z with `ok`; `/api/version` agrees with `main`'s merge commit; the watcher's heartbeat is current (18:51Z) | **Holds**, demonstrated repeatedly |
| It **rolls back a failed update** | `internal/archtest/watcher_test.go` runs the real `update.sh` against scripted fakes and covers a health failure that rolls back and a rollback that also fails. The live rollback was demonstrated on `~/Sepia-scratch` (port 8897) on 2026-11-15, never on production | **Holds on scratch**, by design; production has never needed it |
| A **nightly backup is in R2** | `/api/backups`: `ok`, 2,473,984 bytes, 2026-11-22T08:30:07Z (03:30 board time), with the same size on the 21st and 20th. Helper heartbeat current | **Holds** |

The `covers` backup target has been `skipped` every night: *"nothing to mirror: the covers has no directory yet"*.
That is expected given 0 entries, and it is the visible symptom of the finding in 5.2, not a backup defect.

## 3. Which services the board believes are up

`GET /api/services` on the potato:

| Service | State | Detail |
|---|---|---|
| `main_model` | ok | `deepseek/deepseek-v4.1-flash-1017` |
| `decisions` | ok | Quick pre-read and routing |
| `tmdb` | ok | Films and TV |
| `open_library` | ok | Books |
| `polaris` | **not_configured** | Web lookups: S15, not built |
| `database` | ok | Your entries and drafts |
| `r2` | ok | Last run 2.5 MB |

`GET /api/search` answers `{"code":"not_implemented"}` and Settings returns `{"preread":true}`. Both are honest:
S16 and S15 are phase 2. No screen claims a feature that is not there.

## 4. The invariants, and what actually enforces each

A phase gate is the place to check the rules that must never bend, in the code rather than in the log.

| Invariant | Enforced by | Test |
|---|---|---|
| **Nothing saves without the confirm tap** | `store.ConfirmDraft` is the only path that writes a non-draft entry, behind a version check | `TestConfirmDraftIsTheOnlyCommit`, `TestConfirmDraftRefusesAStaleVersion`, `TestAnUnratedDraftCannotBeConfirmed` |
| **Nothing is ever hard-deleted** | The database's own triggers, in the migrations, not just the repository layer | `TestVerifyTriggersOnAFreshStore`, `TestRawDeleteIsRefusedOnEveryProtectedTable`, `TestDoctorTriggerListMatchesTheSchema` |
| **My words are mine** | Spans are located in her messages; a model's `completion`, rating and verdict are refused unless contained in her text | `internal/agent` containment tests, `internal/session` guards |
| **No live provider in CI** | `make guard` (gitleaks, embed placeholder, fuzz seeds, no live provider in tests) | `guard` job, green |
| **Bounded everything** | The limits table in `STANDARDS.md`, 43 bounds, one row each | The per-limit tests in `store`, `providers`, `server`, `agent`, `backup` |
| **Pure Go, no cgo** | `nocgo` job builds every package with `CGO_ENABLED=0` | `nocgo`, green |
| **One dependency per slice, with an entry** | `STACK.md`; T9 makes an unlisted dependency a build failure | `archtest` dependency rule, `deps` job |

Nothing in the review found an invariant broken. The findings below are all "specified, not yet exercised", not
"specified and violated".

## 5. What is not true yet

Ordered by what it costs her.

### 5.1 No review has been saved on the potato (the phase's own exit condition)

Her only real draft stopped at the rating question on 2026-11-15 and has waited there for seven days. Two readings,
and the first is the one worth acting on: the phase's exit condition is unproven until a real entry exists, and a
card that asks for stars and then does not get an answer is a card she did not finish. That is the cheapest live
check in the project and it is the thing this gate should not close without.

### 5.2 The cover pipeline is built, tested, and has no caller

`Finder.Cover` in `internal/providers/covers.go` downloads, caps, decodes and stores a cover with a 180-day refresh,
and it is exercised thoroughly in `covers_test.go`. **Nothing outside its tests calls it.** The agent wires `Lookup`
and `Fetch` (`internal/agent/tools.go:202`, `:406`), never `Cover`, so `domain.Work.CoverPath` is never set, and
`server.wireWorkOf` therefore never emits a `cover_url`. `Cover.svelte` falls back to a medium-tinted stand-in, so
nothing looks broken; there are simply no posters, and `data/covers/` has never existed on the board, which is why
R2's covers target is skipped every night.

S06's plan says *"Covers ship in this slice"* (decision 1) and its happy route names "a stored snapshot and cover", yet
S06 is marked `done`. This is the one place the status table is optimistic. The work is small: fetch the cover when a work is fetched, set
`CoverPath`, and let the read-time freshness check do the rest. It needs a decision on the gate-before-cost, because
it is the first thing that writes images to the board's disk.

### 5.3 Phase 0's gate: S01 is unwritten

S01 is `planned`, `docs/spikes/` does not exist, and S15 and S18 are both specified as depending on it. The decisions
the spikes were meant to justify were made in practice (embed, not extension; the pinned model; TMDB terms), and the
code that depends on them is built and green, so this is a documentation debt rather than a live risk. It is still
the gate for phase 0, and the honest way to close it is to write the findings from what the shipped code now proves,
splitting off S01.6 (vision) as the one spike that is genuinely still open, which is S18's first step anyway.

### 5.4 Issue #31 is not in the status table

The three-shelf Library layout and the undated tail ordering (wanted rows sort by id, not by when they were added)
are decided in #31 and appear in no slice. Until it is a row, a future agent will find the ordering odd and fix it
inside another slice. The table needs the row.

### 5.5 Small things

- **`govulncheck` is not missing.** It is a step inside the `go` job, and that job ran and passed on `main` on this
  push, so a vulnerability verdict exists. `deps` is a different check (dependency review, PRs only) and its absence
  is by design. This bullet asserted the opposite when the file was first written; the review caught it, which is
  the point of checking a review against the tree rather than against the log.
- `sepia restore` has passed its drill; it has never been run against real reviews, because there are none.

## 6. What I asked Autumn for (answered below)

1. **Clear G8 for phase 1, or not?** My reading: the engineering is there and the invariants hold, but the phase's
   own headline condition has never been demonstrated on the board. Finishing the draft (below) is a ten-minute
   check that makes this a clean yes.
2. **The live check only you can do.** Open Drafts, tap the waiting row, answer the rating and confirm; or log
   something small from `/new`. Then `/api/entries` must show one entry and the library must show it. If the card does
   not let you finish, that is a real defect and the most valuable thing this gate can find.
3. **The cover finding (5.2): fix now, or schedule it?** It is small, self-contained and adds a write path to the
   board's disk, so I would not fold it into another slice silently. My recommendation: a short follow-up slice after
   S01.6, when the vision spike's own image handling is known, so the two image paths are designed together.
4. **S01 (5.3): write it up from the shipped code and split out the vision spike, or leave it?** Recommendation:
   write it up while starting S01.6, because the vision spike is the one part that cannot be recorded from code.

## 7. What happens next

Whichever way 6.1 goes, the order I would argue for is unchanged from the last handoff, with one insertion:

| Order | Work | Why |
|---|---|---|
| 1 | **S01.6 vision spike, then S18** | The reason the project exists. The biggest unproven risk. Needs G2 and G7 before any image leaves the device |
| 2 | **Cover wiring (5.2)** | Small, closes the one gap where a `done` slice is optimistic, and the R2 covers target stops being skipped |
| 3 | **S16 search and chat** | Daily-use value, no keys; the search box is disabled today |
| 4 | **S15 web fallback** | Polaris is `not_configured` on the board until this ships |
| 5 | **S17, then S19** | Repeatable template, then rule 7: the archive outlives the app |

S01's written findings (5.3) run *inside* step 1 rather than as a separate sitting.

## 8. Risks

- **The one real session is stale, not lost.** It is `active` and resumable, and nothing expires it (that is the
  Drafts screen's promise). But it is the only data on the board, and a fresh database after any restore drill would
  not have it: the last backup before it exists, so keep the drill pointed at a scratch directory.
- **Copying the live database anywhere near a test** would break invariant 10. The review read only counts and
  identifiers from `GET /api/…`; no review text was read, stored or quoted anywhere above.
- **Do not read "phase 1 is green" from CI alone.** CI runs against `internal/world`; the board runs against the real
  model, the real providers and R2. The three live conditions in section 2 are the only phase 1 evidence that means
  anything, and one of them is missing.

## G8 cleared, 2026-11-22

Autumn answered the three questions put to her, in this order and these words in substance:

1. **G8 is cleared.** She closed the gate rather than wait for a real entry to exist first. The review's finding stands
   as written: the phase's headline condition has still never been demonstrated on the board, and the live check in
   6.2 is now a follow-up she owns rather than a gate that blocks phase 2. Phase 2 may start.
2. **The cover finding (5.2)** is recorded for a follow-up slice rather than fixed in this change; the recommendation
   in 6.3 stands (after S01.6, so both image paths are designed together).
3. **S01 (5.3)** gets its write-up alongside the vision spike: S01.6 is answered and written up in the stacked change,
   and S01.1 to S01.5 are still owed. The slice table says so rather than implying the whole slice is done.

Nothing in this branch changed after the answer: the review above is the record of what was true when the gate was
answered, and the two gaps it names are open work, not silent. The draft on the potato is still waiting on a rating.
