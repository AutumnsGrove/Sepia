# S18. Screenshot import

## Purpose

Bring the Fable library in, as far as it can be brought. Fable has no export, and a written review is not viewable
outside the screen it was written on (S01.6, decided 2026-11-22), so what a screenshot can carry is the **work**:
title, creator, the rating as shown and the date it was read. Attach a screenshot in the chat, have the pinned model
read those four things off it, and turn them into a normal draft she reviews and confirms. One session per title, any
number of images. The originals are kept as the entry's source.

**What this slice is not.** It does not import review bodies, because they are not in the image and no screen shows
them; the body remains hers to type or to leave empty. That is why nothing here takes the import route into the
body-span machinery: `transcript_json` records the four fields, and no entry's body is built from a model's reading.

## Depends on

S11 (the session UI), S08 (the agent), S04 (`media`), S14 (backups, so the originals are protected), and the
S01.6 finding ([`../spikes/01.6-vision.md`](../spikes/01.6-vision.md)). Phase 3.

**Decided 2026-11-22 (T10, Autumn).** The finding measured title, creator, quarter-star rating and date off four real
screenshots, exactly, and found no review body to measure; she then reported that a written Fable review is not
viewable outside the screen it was written on. Her answer: **import the works only.** A screenshot brings the work,
the rating and the date, and her bodies stay where they are. `PLAN.md` and the phase 3 gate say the same thing now.
Two schema fixes from the same finding go with it: `unreadable` must be bounded, and an absent field needs an explicit
value rather than an empty string carrying 90 to 99 confidence.

## Where we start

The composer is the textarea and Send: there is no attachment affordance yet, because a control that is
permanently off teaches that the app is broken (issue #15 removed the placeholder one). This slice adds the
control back with its own reason to be enabled, its own limits and its own accept step (S18.4). The `media` table
and `body_origin = imported` exist in the schema. The decisions layer takes text only.

## Settled

- **One session per title**, up to six images of ten megabytes each; the originals are kept and mirrored to R2.
- `read_source_image` returns title, creator, rating and date as shown, and nothing else. No body text is
  transcribed, because there is none to transcribe. The screenshot sits beside the card, and fields read from an
  image carry a marker and need an accept tap before they count as her own.
- A message with images and no text defaults to the `import_review` intent in code. The decisions layer reads
  only the accompanying text.
- Entries record `imported_from`. `body_origin = imported` stays in the schema and is **not** used by this slice,
  because no body is imported; an imported entry's body starts empty and is typed if she wants one. Bank questions
  are not asked for an import unless something required is missing.
- **A finished entry may have no body** (decided 2026-11-22, `PLAN.md` 2 and [`../ENTRY_CARD.md`](../ENTRY_CARD.md)
  2). Without that, this slice could not save anything: an imported row is a rated finish with no body, and the save
  guard used to refuse exactly that shape.
- Images go to the pinned model through OpenRouter only after the user attaches them.

## Scope

1. **S18.1 Upload.** `POST /api/sessions/{id}/media`: type and size sniffing, count and size limits, a safe
   filename, EXIF stripped, a decompression-bomb guard, storage under `data/media/`, and a `media` row.
2. **S18.2 `read_source_image`.** The tool with a strict output schema — title, creator, rating, date, a
   confidence per field and a **bounded** `unreadable` list — with a per-image result stored as `transcript_json`.
   An absent field says so instead of returning an empty string with high confidence. Hostile text in an image is
   data, never an instruction.
3. **S18.3 The import flow.** The pipeline from images to a draft: the four fields, title and creator matched to a
   work, the revisit check, markers on image-read fields, and `fill_gap` for a missing date or an ambiguous title.
   No spans, no body.
4. **S18.4 UI.** The attach tray, the reading state, the "so far" panel, the source block on the card with the
   screenshot beside the transcription, and the accept strip (page 02 frame C, page 07 frames 3 and 4).
5. **S18.5 Safety and cost.** Image tokens counted in the ledger, a per-session image cap, and what leaves the
   device stated in the UI before sending.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | A screenshot becomes one draft with the work, the rating and the date, and the screenshot kept beside it; ten real screenshots are imported and read back (the operator's live check) |
| Bad | An oversize or mislabelled file, a decompression bomb, a path-traversal filename, a screenshot that is not a review, a screenshot containing instructions, a rating read as a different value from the one drawn, an absent field reported as a confident value |
| Sad | The model fails or times out on image two of two: the first is kept and the user can retry the second; a disk-full upload is refused cleanly; a restart mid-import resumes from the stored media |

**Fuzz:** the upload sniffing and the transcript decoder. **Privacy:** the decisions fake never receives an
image; the world's request log shows images only in the model call.

## Gates

**G7** before any image is sent to a model: what leaves the device, the limits, the stored copy, and the
operator's yes. **G2** applies to any real screenshot used in testing.

## Open questions

- Whether to offer "confirm all in this list" for a large migration. It matters more now: her screenshots are list
  views with four or five works on each, so one image can produce several drafts. Each still needs its confirm tap
  by default (BUILD.md invariant 2), which makes a 46-book library a lot of taps.
- Whether a list screenshot becomes several drafts in one turn or one draft per tap. The finding measured the model
  reading a whole page correctly, which is what makes several possible.

## Done when

A Fable screenshot becomes a saved entry carrying its work, rating and date, with the screenshot beside it; ten real
screenshots have been imported and read back correctly; and no image or review text appears in a log or the repo.
