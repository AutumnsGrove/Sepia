# S11. Logging flow UI

## Purpose

The screens you use every time: the session with its transcript, every interaction, and the entry card in all
its states. This is the heart of the product, and the mockups are its spec.

## Depends on

S10 (components, client, socket machine) and S02 (golden examples). Runs against the dev stack, and against the
real server once S09 lands. **S12 and S13 build on it.**

## Where we start

Mockup pages 01, 02, 06 and 07 ([`../../mockups/`](../../mockups/)) and [`../ENTRY_CARD.md`](../ENTRY_CARD.md).

## Settled

- No chat bubbles: the transcript is plain blocks, activity rows are quiet lines, and the agent speaks in cards.
- Every interaction kind has a designed card: `choose_medium`, `choose_candidate`, `describe` (tabs of chips with
  counts, dashed hints, a "you mentioned this" note), `question_form`, `revisit_choice`, `fill_gap`,
  `lookup_failed` (Try again, Ask Polaris when available, Log without details).
- The entry card: blocks in the order Work, Rating, My words, Facts, Answers, Tags, Why; the markers for
  verified, unverified and image-read; tap-to-edit; the left-out section; the fidelity warning; the sticky
  confirm bar and the saved state.
- System states: offline strip, back-and-caught-up, working with Stop, one message queued, error with retry,
  the pre-read-down notice.

## Scope

1. **S11.1 Session screen.** Transcript blocks, activity rows with the live pulse, the status strips, the composer
   with Stop and the queued state, the attach button (disabled until S18).
2. **S11.2 Interactions.** Each kind above as a component driven by an `interaction_request` and answered with an
   `interaction_response`, with hints dashed and nothing pre-selected.
3. **S11.3 Entry card, draft.** All blocks and states; the rating control; edit in place with `draft_edit` and
   its version handling; the unverified marker and source panel (live in S15); Accept all; Left out; the
   fidelity warning; the "why" sheet; required-to-confirm shown beside the button.
4. **S11.4 Entry card, saved and edit.** The saved state, Edit opening a draft revision with the "what will
   change" diff, and Log again.
5. **S11.5 Resilience.** Resume mid-reply, a stale version, a rejected action with its reason, a second device.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | The Dune flow through a real socket against the dev stack: split read, pick, Describe, draft card, confirm, saved; each mockup frame has a Playwright state with a baseline and an axe pass |
| Bad | A stale version shows the current card and says why; a message over the limit is refused in place; a hint is never pre-selected; Save is disabled without a rating and without words, with the reason shown; hostile text in a title renders as plain text |
| Sad | Offline mid-flow and back, a model error with Try again, a lookup failure with its three ways on, a queued message, the pre-read down notice, a killed backend restarted mid-session |

**Fidelity to the mockups** is asserted per frame on a phone viewport. **Visual checks are looked at by a human**
(or by the agent rendering and reading the image), not trusted to a green diff alone.

## Decisions

Recorded with Autumn on 2026-11-09, before the slice started.

1. **Routes.** `/new` creates an entry session (`POST /api/sessions`) and replaces to `/s/{id}`; the session
   screen is `/s/[id]`. Home, the library and the entry page stay S12's.
2. **One screen component.** `SessionScreen` is pure and state-driven: it takes a `SessionState`, the
   connection state, the outbox and a set of callbacks, and renders every frame. The route owns the socket and
   the store; the gallery renders fixtures through the same component, so a baseline and the real screen cannot
   drift apart.
3. **Copy comes from the contract.** Every error and notice sentence is `copyFor(code)` from
   `web/src/lib/contract/codes.ts`; the screen never invents a code. The sentences this slice adds (not per
   code) are the table below, for Autumn to review at the handoff.
4. **A lookup failure is composed, not coded.** Its sentence is built from the interaction payload's `service`
   and `reason`, so TMDB and Open Library can say what actually happened.
5. **`reopen` (approved 2026-11-09).** One new client message, `{entry_id}`, opens a draft revision of a saved
   entry through the engine's `StartRevision`, the same path the revisit choice uses. The saved card's Edit
   sends it; Log another starts a new session. S12's Edit and Restore as draft reuse it.
6. **The diff is the client's.** "What will change" is computed between the saved entry (fetched over REST) and
   the revision draft, in card order, listing only fields that differ.
7. **The message limit is 8,000 characters** (`STANDARDS.md` limits table). Over it, the composer refuses in
   place with the `message_too_long` copy and sends nothing.
8. **Attach is disabled until S18** with a plain reason. The import frames on pages 02 and 07 are out of scope.
9. **A split dev scenario.** `SEPIA_DEV_SCENARIO=split` scripts a split medium, a candidate choice, Describe and
   a question form against the invented sample, so the end-to-end suite drives the whole flow offline. The
   default script stays as it is.
10. **Unverified fields are built, not fetched.** The marker, source panel and Accept all read
    `draft.unverified` and `draft.fidelity`; the web lookup that fills them live is S15's.

### Copy this slice adds

| Where | Words |
|---|---|
| Offline strip | "Offline. Reconnecting." |
| A message typed offline | "Will send when you are back online" |
| Back and caught up | "Back online. You did not miss anything." |
| A message sent mid-turn | "Queued, sends when this finishes" with Cancel, and "One message is already waiting." below |
| Error box | `copyFor(code)`, with Try again when `retryable` |
| Lookup failure | "{service} did not answer in time. Your message is saved as a draft." plus Try again, Ask Polaris when available, Log it without details |
| Save is off | "Add a rating to save", "Add a few words of your own to save", or both, in that order |
| A required field | the "Needed" marker beside the Rating label |
| Fidelity warning | the draft's `fidelity.warning`, else the contract's `fidelity_drops_opinion` copy |
| Editing a saved entry | "You are editing a saved entry. Saving keeps the previous version." |
| Unverified marker | "From the web, unverified"; image-read: "Read from the image" |
| Accept row | "{n} fields from the web"; image-read: "Rating and date were read from the image" |
| Source panel | the source URL and its quoted snippet, with Accept and Edit |
| Why sheet | "How this was read", odds with percentages, tools in order, costs split into consumed and plan-covered, and "An allowance is not money spent." |
| Saved | "Saved to your library" |
| Left out | "Left out ({n})" with Include on each segment |
| A stale version | `copyFor('stale_version')`, and the current card is shown |
| Attach | disabled, "Screenshots arrive later" |

11. **Two controls wait for S12.** Tag editing and Edit on an unverified field are not built: `draft_edit` carries
    the entry's fields, and a work's title and year are a different write with its own accept rules. The card shows
    tags read-only and the source panel offers Accept and Accept all. Both belong with the entry page, where the
    work is shown alone, or with a later patch field.
12. **The dev stack has a fault switch.** `SEPIA_DEV_FAULT=chat|lookup|preread` makes one outside service
    misbehave, so a screen test can see a real error and a real way back rather than a fixture.

## Gates

None.

## Open questions

None. The copy table above is confirmed by Autumn in review, before the slice is marked done.

## Done when

The operator can log a review end to end on a phone against the dev stack and it matches the mockups state for
state; every system state in pages 06 and 07 (except import) has a test.
