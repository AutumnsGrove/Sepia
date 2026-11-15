# S14. Backups

## Purpose

Protect the first real review from the day it exists. A nightly SQLite snapshot, a mirror to R2 through the
`rclone` remote already on the box, receipts the app can show, and a restore that has actually been performed.
Local-first: R2 is a mirror, never the source of truth, and a failed mirror is a warning, never a lost review.

## Depends on

S13 (it runs where Sepia runs) and S04 (the `mirrors` table). It is the last slice of Phase 1.

## Where we start

The `rclone` remote `r2:` is configured for Vaultwarden (`vaultwarden-setup/vaultwarden-backup.sh` is the
precedent). The bucket `r2:sepia-backups` does not exist yet.

## Decisions (answered 2026-11-15, before starting)

- **A host timer schedules the backup**, not an in-app scheduler. It is the Vaultwarden precedent: a wedged
  container must not be able to stop backups silently, and the host already has the pattern for a nightly job.
- **Retention is seven daily and four weekly**, both kept in R2. A week of dailies alone loses a mistake found on
  the eighth day; two weeks of dailies buys little over one plus the weeklies.
- **The existing `r2:` remote is tested first.** If its token is account-wide, `r2:sepia-backups` is created with
  it; if it is scoped to the Vaultwarden bucket, Autumn makes a token for the Sepia bucket and no agent handles the
  secret. **Tested 2026-11-15: the token is scoped to `vaultwarden-backups`.** It cannot list buckets, create
  `sepia-backups` or read it. A token scoped to the Sepia bucket (or an account-wide one) is a prerequisite for this
  slice; the bucket itself can then be created with that token.

### Decisions at slice start (2026-11-15)

- **R2 access, done before any code.** The existing `r2:` token was tested and refused (403 on `ListBuckets`), so
  the bucket and a scoped token were made on 2026-11-15: `sepia-backups` created from the operator's machine with
  `wrangler` (location ENAM, standard storage class, no public access), and an R2 API token scoped to that bucket
  alone with Object Read & Write. The token lives in a **second** rclone remote, `r2sepia`, in the potato's
  `~/.config/rclone/rclone.conf` (mode 0600, backup kept beside it). `r2:` is untouched, so the Vaultwarden backups
  keep working. **No agent read the key**: a template was opened for Autumn, she pasted the two values, and the
  section was appended unread; the only things checked were the value lengths and a write/read/delete against the
  empty bucket.
- **The host runs the backup**, which follows the reasoning above and was confirmed with the operator: the timer,
  the snapshot and `rclone` are all on the potato, as the operator, outside the container. Consequences: the token
  never enters the container and the distroless image stays free of a 60 MB `rclone` binary; a wedged container
  cannot stop a backup; and the snapshot uses the host's `sqlite3` `.backup` (the online backup API) against
  `data/sepia.db`, which was measured on the live board at 13 ms for 274 KB with `PRAGMA integrity_check` clean and
  row counts equal to the source while the container was writing. The cost, accepted: the backup logic is shell and
  is tested the way `update.sh` is, against scripted fakes in `internal/archtest`, not by Go unit tests.
- **Receipts come back through a file, not through the container.** The script appends one JSON line per mirror to
  `data/backups/receipts.jsonl`; the app imports unimported lines into `mirrors` (`INSERT OR IGNORE`, id is the run
  id plus the target, so a re-import cannot duplicate and a run that finished while the container was down is not
  lost). A receipt the app cannot parse is counted and skipped, never guessed at.
- **`Back up now` reuses the signal directory.** The app writes `backup-requested` into `update-signal/` (temp file
  plus rename) and a `sepia-backup.path` unit fires the same script; the directory therefore carries update and
  backup requests, and `docs/UPDATE.md` says so. No second mount, and the backups directory stays pure data, which
  keeps the `rclone sync` excludes to none.
- **The screen is a page, not a row.** The mockups only carry the Settings row, so S14 adds a `Backups and export`
  screen in the page-05 style (last run, snapshot size, R2 state, an error line, `Back up now`, recent runs), and the
  mockup is extended first so the operator can look at it before the Svelte page is written.
- **Two targets per run, two receipts.** `data/backups/` mirrors to `r2sepia:sepia-backups/db/` and `data/covers/`
  to `r2sepia:sepia-backups/covers/`. A failed mirror is a failed receipt on that target only; the other still runs.
- **Retention:** seven daily snapshots on the board and in R2, plus four weeklies in R2 (the first successful run of
  each ISO week). The weeklies are pruned oldest first, and a prune never removes the snapshot that was just taken.
- **A snapshot failure gets its own receipt target, `local:snapshot`.** It shares neither the id nor the target of
  the mirror, so a night where the snapshot failed and the mirror of yesterday's good files succeeded reads as one
  failure and one success rather than one ambiguous row. The screen shows the failure; the database shows both.
- **The heartbeat is the run itself.** No sixth timer: the script writes a timestamp at the start of every run, and
  the app calls the helper stale after 26 hours (a day plus two hours, so one slow morning is not an outage). "Never
  installed" and "gone quiet" stay different sentences, which is the whole reason for a file at all.
- **The Services row reads the newest receipt, not R2.** It says "Last run 4.2 MB" or "Last run failed", and it never
  makes a live call: the receipt is the evidence, and a receipt is why one is written. The clock is deliberately left
  out of that row, because the timer fires in board time and the container is UTC, and a wrong hour is worse than
  none.
- **A restore restores the database.** It refuses a tampered or truncated snapshot, refuses a target that holds
  anything, proves the copy it wrote, and with `--compare` proves the restored library equals the source row for row.
  The covers are objects in R2, copied back with one `rclone copy` line in `docs/INSTALL.md`; a missing cover is a
  placeholder, not a lost review. The drill is documented there and is the operator's to run.

## Settled

- Nightly at 03:30: a SQLite snapshot into `data/backups/` (rotating), then `rclone sync` to
  `r2sepia:sepia-backups/` (needs its own remote: the `r2:` token is Vaultwarden-only, see Decisions).
- Covers and media are mirrored too; receipts go in `mirrors`; failures show as a warning in the app.
- Soft delete means every backup holds every copy; there is no purge.
- A restore is an offline command, never an HTTP endpoint (a database file cannot safely be swapped under a
  running connection).

## Scope

1. **S14.1 Snapshot.** An online backup (the SQLite backup API or `VACUUM INTO`), an integrity check on the
   result, rotation, and a size and age policy.
2. **S14.2 Mirror.** `rclone` invoked with a timeout and bounded output, partial-upload detection, a receipt row
   for every run (`target`, `status`, `bytes`), and a clear failure state.
3. **S14.3 Schedule.** *Open:* a host systemd timer calling the app, or an in-app scheduler. Proposed: a host
   timer, the Vaultwarden precedent, so a stuck container cannot silently stop backups.
4. **S14.4 The screen.** The Backups part of "Backups and export": the last run, the size, any error, and a
   Back up now button.
5. **S14.5 Restore.** `sepia restore` into a scratch location, with a verification that the restored library
   equals the source, and a documented drill.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | A nightly run produces a verified snapshot, mirrors it, and records a receipt |
| Bad | A corrupt snapshot is detected and never mirrored as good; `rclone` output is bounded and cannot inject into logs; a restore of a tampered snapshot is refused |
| Sad | R2 unreachable, an auth failure, a partial upload, a full disk, a snapshot taken mid-write, the process killed during a run: each ends in a visible state and no corrupted backup replaces a good one |

**Property:** restore then compare equals the source library. **The drill:** a restore exercised on a scratch
copy is part of "done".

## Gates

**G6** before the first real review is stored on the potato: a backup that reached R2, a restore tested on a
scratch copy, and the receipts screen working.

## Open questions

None. The scheduler and retention questions were answered on 2026-11-15 and are recorded under Decisions. The R2
access question is answered the same day: test the existing remote before asking for a new token.

## Done when

A backup has reached R2 and appeared in the app, a restore has been performed for real, and the operator has
cleared G6.
