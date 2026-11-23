# CLAUDE.md

Guidance for Claude Code (or any agent) working in this repo. This file is checked in and takes precedence over
generic instructions for work done here.

## What this is

Sepia is a private, single-user review journal for every medium (films, TV, books, games, albums and more). The
user types what she thought, an agent finds the work, extracts her own words, asks a few light questions, and she
confirms one entry card. It is a Go service with an embedded SvelteKit frontend, run on a Le Potato board over
Tailscale, and it is a sibling of Polaris, Ivy, Warbler and Meridian in `~/Documents/Projects/`.

**Sepia listens on port 8893**, loopback only (`127.0.0.1:8893`) — `tailscale serve` is the only way in,
and the public-to-her address is `https://lepotato.taild95077.ts.net:8893`. The port is reserved and was
verified free on 2026-10-30; 8892 belongs to Warbler and 8894 to Coffer. Any probe, health check or
Meridian tile uses `http://localhost:8893/healthz`.

## Status

**Planning only. Nothing is built.** The repo holds the plan, the specs, the clickable mockups and the build
plan. Code starts at slice S00.

## Start here, in this order

1. [`docs/BUILD.md`](docs/BUILD.md): the standing brief. Invariants, settled decisions, traps, gates and triggers.
2. [`docs/STANDARDS.md`](docs/STANDARDS.md), [`docs/TESTING.md`](docs/TESTING.md), [`docs/CI.md`](docs/CI.md).
3. [`docs/slices/README.md`](docs/slices/README.md): the order, the dependency graph and the status table.
4. Your slice in `docs/slices/`, then only the specs it links.
5. For any UI, open the matching page in [`mockups/`](mockups/index.html). The mockups are the UI spec.

`PLAN.md` is the source of truth for what Sepia is. If two docs disagree, stop and ask (trigger T11).

## About the user

The user is Autumn and always uses **she/her**. Use them in commit messages, comments, docs and summaries, and
never guess otherwise.

## Rules that are never relaxed

- **Test first, on three routes:** happy, bad and sad. Watch the test fail for the right reason. Every bug gets a
  regression test seen failing without the fix. Fuzz anything that parses untrusted input.
- **My words are mine.** A saved body is made only of text from the user's own messages. No model output is saved
  as an opinion, a rating or a quote.
- **Nothing saves without her confirm tap.** Nothing is ever hard-deleted; the database enforces it.
- **No real reviews, screenshots or keys** in the repo, a fixture, a test or a log. The repo is public.
- **No live provider in tests or CI.** Live checks are hers.
- **Stop at gates and triggers** ([`docs/BUILD.md`](docs/BUILD.md) 6) with a committed handoff file and a message
  beginning `STOP: this needs Autumn:`.

## Conventions

- Commit messages: present tense, a subject under 50 characters, a body that says why. No emojis anywhere.
- User-facing copy is plain and short, written for someone new to the vocabulary. No em dashes in UI copy.
- Pure Go, no cgo (except `go test -race`). Standard library first; a new dependency needs a `STACK.md` entry.
- Update the docs, the plan and the slice status in the same commit as the code.

## Commands (built in S00)

```bash
make check      # format, vet, lint, race tests, a short fuzz smoke, frontend checks, guard
make test       # go test -race ./... (cgo on for the race runtime only)
make e2e        # Playwright against the real binary and the world, on a phone viewport
make gen        # regenerate types from api/ (nothing to generate until S02)
make web        # build the frontend into a scratch directory and into internal/webui/dist
make dev        # the real service against the in-process world, offline
make eval       # the S08 live corpus against the real model (operator's check; needs SEPIA_OPENROUTER_API_KEY)
make guard      # embed placeholder, fuzz seeds, no live provider in tests, gitleaks
```

The frontend lives in `web/` and needs `pnpm install` once. `make web-check` runs svelte-check,
ESLint, Prettier and Vitest.

## Checking UI by eye

There is no Chrome on this machine, but Brave is Chromium. To render a page headless and look at it:

```bash
"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" --headless=new --disable-gpu \
  --hide-scrollbars --virtual-time-budget=2000 --window-size=1200,1000 \
  --screenshot=/tmp/shot.png "file://$PWD/mockups/index.html"
```

A green pixel diff is not enough. Render it and look at it, on a phone viewport.

## Pull request reviews

`pi-review` reviews every pull request. It checks out the merge commit, installs, and runs the code, so it
takes minutes rather than seconds. Its check is the **`review / review`** row of `gh pr checks`.

**Do not push while `pi-review` is reading.** The `pi-review` workflow cancels its own superseded run, so a push cancels the review in flight, and a cancelled run has already
read most of the pull request and spent most of what it was going to spend. Pushing to be fast is the
expensive choice, not the cheap one.

Read the `review / review` row and act on the state it reports:

| `review / review` on this head                   | What to do                                                                                                     |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| pending                                          | **Wait. Do not push.** A push cancels it and the wait starts over.                                              |
| skipped                                          | A draft, or a branch that is not in this repository. A draft can be marked ready; a fork is never reviewed.      |
| a review for this head                           | Push **once**, with every finding from it. One push per finding restarts the cycle and pays for the read again.  |
| failed, or cancelled by a push that was not yours | It will not post for this head. Say so on the thread, then push.                                               |
| still pending, and it has no start time | The run is queued for a runner, not late. GitHub enforces the 33-minute job timeout itself, so a job that is *running* cannot sit past it — it ends cancelled or failed, which is the row above. A run with no start (`gh run view <id> --json status,startedAt`) is waiting and will start, so wait. |

A green `review / review` tick means a review was posted — the job exits non-zero when it cannot post — so
read the review rather than trusting the tick. A red check from CI is the one exception to all of the
above: fix it and push.
