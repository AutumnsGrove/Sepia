# S10. Frontend foundation

## Purpose

The base the three UI slices stand on: the SvelteKit app, the design tokens, fonts and icons from the mockups,
a gallery of every base component in every state, the typed client and **the socket state machine**, and the
Playwright harness with visual baselines. Built against the S02 contract and the world, so it does not wait for
the backend.

## Depends on

S00 and S02. **Runs in parallel with S03 to S09** (the frontend stream). S11 and S12 need it.

## Where we start

Static mockups in [`../../mockups/`](../../mockups/) with the real CSS in `sepia.css`; the golden examples from
S02; the dev stack from S00 and, later, S09.

## Settled

- SvelteKit with Svelte 5 runes, `adapter-static`, TypeScript strict, pnpm, pure CSS custom properties from the
  mockups' tokens, no Tailwind and no CSS-in-JS ([`../STANDARDS.md`](../STANDARDS.md) 5).
- Dark only. Lexend and Fraunces as bundled woff2, Lucide through `@lucide/svelte`, fill and stroke as CSS
  variables.
- One typed client; components never touch `fetch` or the socket. Types come from the generated contract.
- The socket machine: connect, `hello`, replay from `last_seq`, an offline outbox keyed by `client_msg_id`,
  heartbeat, backoff of 0.5 to 15 s with jitter, an immediate attempt on `visibilitychange`, a reload on a
  `server_version` mismatch ([`../PROTOCOL.md`](../PROTOCOL.md) 1 and 6).
- State is never carried by colour alone; reduced motion is respected; touch targets are at least 44 px.

## Scope

1. **S10.1 App and tokens.** Scaffolding, the token file extracted from `mockups/sepia.css`, fonts, icons,
   the global styles, the `viewport` and PWA manifest (no service worker yet).
2. **S10.2 Base components.** Button, Pill, Chip (pick, hint, on), Switch, Tabs, Statusbar, Banner, Errbox,
   Interaction card, Cover, Cloud, List row, Rating control (drag across, snap to quarters, plus and minus, the
   number always visible), each in every state of the mockups.
3. **S10.3 Gallery.** A dev-only `/_gallery` route showing every component in every state. It is the unit of
   visual regression: one baseline per state.
4. **S10.4 Client and socket machine.** The typed REST client and the WebSocket client as a tested state
   machine with a fake socket in Vitest.
5. **S10.5 Stores.** A session store derived from events (transcript blocks, activity rows, open interactions,
   the draft snapshot), and a drafts and recent-entries store; all derived, never mutated by components.
6. **S10.6 Playwright harness.** Boot the dev stack and the world, a phone viewport as the default, a helper to
   load a named golden state, the screenshot baseline tooling, axe, and a trace on failure.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | Every gallery state matches its baseline and passes axe; the socket machine connects, resumes and flushes the outbox in order |
| Bad | An event the client does not know is ignored and logged; a malformed frame does not crash the app; a message over the limit is refused in the composer before sending |
| Sad | Offline then online resumes with no lost or duplicated text; a version mismatch in `hello` reloads; a stalled heartbeat reconnects; a dropped socket mid-reply catches up; reduced motion removes the pulse and the progress animation |

**Fixtures:** the S02 golden examples drive component tests. **Budgets:** compressed initial JS and CSS bytes are
asserted.

## Gates

None.

## Open questions

- Whether a minimal service worker is wanted for an installable shell (proposed: no, until a real need appears,
  because caching interacts badly with updates).

## Done when

The gallery is complete and baselined, the socket machine's transitions are all tested, and a screen can be
built from a golden example alone.

## Decisions (recorded 2026-11-08, while building)

1. **The gallery is built into the app, unlinked.** The plan says dev-only, but the visual baselines run against
   `cmd/sepia-dev` and the binary it embeds, so a route that only exists in `vite dev` could never be baselined.
   It is a lazy route, linked from nowhere, and costs one chunk in the bundle.
2. **The tokens test reads every `:root` block in the mockups.** Reading only the first block let a token the app
   did not define pass, because the mockups declare `--imageread-soft` beside the card states rather than at the
   top. The app now defines it and the test would fail if either side drifted.
3. **The socket machine owns no DOM.** A `connect` function is injected and defaults to `new WebSocket(url)`, so
   the whole transition table is tested in Node with fake timers. The page binds `visibilitychange` to `wake()`
   and `beforeunload` to `stop()` in S11.
4. **The resume point is the highest `seq` seen, and a lower one is dropped.** A hello with `full_state: true`
   moves it to `head_seq`, because the snapshot in the hello is the state and the compacted events no longer
   exist.
5. **The reactive stores are `$state` wrappers added with the screens.** S10 ships the pure reducers and the
   machine, tested; S11 and S12 own the wrappers, so no screen is written against an untested one.
