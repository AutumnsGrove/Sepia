# Notes for the reviewer

Guidance for the reviewer with a shell. That is `pi-review` in CI, and it is also `claude-review`
since it was given one (2026-11-26). Read `CLAUDE.md` and `docs/BUILD.md` first; this file says only
where your turns are worth spending.

Both copies of `CLAUDE.md` and this file are lifted from the **base** commit into the reviewer's
runner, because a pull request that could rewrite its own reviewer's instructions would be reviewing
itself. If the diff changes `docs/BUILD.md`, `docs/STANDARDS.md`, `docs/TESTING.md` or `docs/CI.md`,
read `git show <base>:/<path>` before believing the changed copy.

## When both readers run

`pi-review` reads the diff line by line in its own job. While `claude-review` reads this pull request
alongside it, do not spend turns on any of this. It is the whole of what the other reader does, and it
reads faster than you do.

- `docs/BUILD-LOG.md` and docs prose, and whether the paperwork a change owes is present.
- Whether a slice plan or spec row exists, and whether a test name cites it.
- Test titles, and "this title claims more than its assertions check".
- Workflow YAML: permissions, action versions, timeouts, concurrency groups.
- Style, structure, "consider extracting", speculative performance.

It has no shell. You do. That is the difference worth paying for, so spend your turns where it applies.

## When you are the only reader

While `pi-review` is disabled, `claude-review` is the only reader, so it does that list as well as
this one. Nothing else is looking at the paperwork, the specs or the workflow YAML, and skipping them
is a silent gap rather than a saved turn.

One thing neither reader can do: this reader cannot review a pull request that edits its own workflow
file. GitHub validates `.github/workflows/claude-code-review.yml` against the default branch and the
action skips, leaving a **green** check that reviewed nothing. Read that YAML by hand and say that the
run skipped.

## What this job can run

The checkout is the pull request's head with full history, `persist-credentials: false`, and no fork
code. Go is installed from `go.mod`; pnpm and Node are the versions `web/package.json` and `ci.yml`
use. So these work here:

- `make test` (`go test -race ./...`, cgo only for the race runtime) and `make coverage`.
- `make nocgo`, `make fmt-check`, `make guard`, and `make fuzz-smoke` (10 s per target by default).
- `go build ./...`, `make build`, and the potato compile the `go` job runs.
- `cd web && pnpm install --frozen-lockfile`, then `make web-check` and `make web`.
- `make e2e` after `cd web && pnpm exec playwright install chromium webkit`. It is slow; time-box it.

`make lint` does **not** work here: this job does not install `staticcheck` or `golangci-lint`, which
CI installs in its own step. Say a lint check was not run rather than claiming it passed.

CI reports as `go`, `web`, `guard`, `drift`, `deps`, `nocgo`, `e2e`, `coverage` and `CodeQL`. A
time-boxed fuzz run is local and nightly (`nightly.yml`), not one of the PR checks.

## Spend them on running the thing

1. **Reproduce the bug the pull request says it fixed.** Work in a copy, because the checkout stays
   untouched: `git worktree add /tmp/sepia-probe HEAD`, or `git archive HEAD | tar -x -C /tmp/sepia-revert`.
   Revert the fix there and run the test that is supposed to guard it. A regression test that passes
   against the code it was written to catch is the most common real defect in this repository. Note
   that a worktree at `HEAD` brings the pull request's own `CLAUDE.md` and docs back: read the base
   copies from the runner's review directory instead.
2. **If the change touches the embedded frontend, build it in the copy before comparing.** `cmd/sepia-dev`
   embeds `internal/webui/dist` at Go build time, so `make web` and a Go rebuild are what make a
   screenshot or an end-to-end run mean anything. A green e2e run against a stale bundle has hidden
   real UI bugs here before.
3. **Run what the pull request claims to have run**, and compare its output with the figures in the
   body. A body that does not match the tree is a finding; a body that matches it is a reason to trust
   the rest. Keep it to the target and the package: a review that a timeout kills posts nothing.
4. **Derive the arithmetic** in any comment that asserts a count, a byte length, a boundary, a budget
   or a timeout. It is a claim you can check by computing it, and an off-by-a-few at an edge is the
   bug that survives both a compiler and a test suite written by the person who wrote the bug.
5. **For every new test, delete the changed lines.** Does it still pass? If it does, the test is not
   testing the change, whatever its title says.
6. **For anything that decides pass or fail, read the exit code and the report shape it reads.** A
   script that reports success on a failed run is the highest-severity finding in a repository whose
   gate is the definition of done.

## Do not spend them on

Style, formatting and naming: the formatter owns those and the checks own the rest. And a finding you
cannot point at with `path:line`, or that no invariant, rule, test or spec settles, is not a finding.

## Reporting

Post only what you ran, with the command and its output. If you could not reproduce something, say so
in one line, or drop it. Two demonstrated findings beat ten plausible ones, and "nothing found" is a
good review when it is true.

## What has broken here before

The recent fixes are one class: **two copies of one fact drifting apart.** A parked turn with a queued
message that reports idle, a turn waiting on her that blocks everything else, an answer read from the
remembered copy instead of the live selection, a remembered answer mapped differently from the live
one.

When a change touches a session, a turn, a card or anything remembered across a reload, ask: where is
the second copy of this fact, and what keeps them the same? A test that only exercises the live path,
or only the remembered one, proves nothing about either.

Two more, from looking rather than reading: a CSS rule on an element beats inheritance (an SVG
`<use>`'s own `fill` and `stroke` win), and a UI change is not verified until it is rendered at
390x844 and looked at.
