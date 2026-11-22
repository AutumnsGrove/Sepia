# S18. Screenshot import

## Purpose

Bring the Fable reviews in. Fable has no export, so the migration is a first-class feature: attach screenshots
in the chat, have the pinned model transcribe them, and turn each into a normal draft the user reviews and
confirms. One session per title, any number of images. The originals are kept as the entry's source.

## Depends on

S11 (the session UI), S08 (the agent), S04 (`media`), S14 (backups, so the originals are protected), and the
S01.6 finding ([`../spikes/01.6-vision.md`](../spikes/01.6-vision.md)). Phase 3.

**Open, and it decides S18's shape (T10, waiting on Autumn).** The finding measured title, creator, quarter-star
rating and date off four real screenshots, exactly, and found no review body to measure. She then reported that a
written Fable review is not viewable outside the screen it was written on, so **body text may not be recoverable from
screenshots at all**. If that holds, what this slice can import is the *work* (title, creator, rating, date) and not
the review, which is smaller than `PLAN.md` describes: the transcription-as-original idea, the body spans and the
fidelity check would all go with it. Do not build S18.3's body path until she has decided, and record the answer in
`PLAN.md` and here. Two schema fixes from the same finding do not depend on that decision: `unreadable` must be
bounded, and an absent field needs an explicit value rather than an empty string with 90-99 confidence.

## Where we start

The composer is the textarea and Send: there is no attachment affordance yet, because a control that is
permanently off teaches that the app is broken (issue #15 removed the placeholder one). This slice adds the
control back with its own reason to be enabled, its own limits and its own accept step (S18.4). The `media` table
and `body_origin = imported` exist in the schema. The decisions layer takes text only.

## Settled

- **One session per title**, up to six images of ten megabytes each; the originals are kept and mirrored to R2.
- `read_source_image` returns title, creator, rating and date as shown, and the text. The transcription is the
  **original** the body spans are taken from. The screenshot sits beside the card, and fields read from an image
  carry a marker and need an accept tap before they count as the user's own.
- A message with images and no text defaults to the `import_review` intent in code. The decisions layer reads
  only the accompanying text.
- Entries record `imported_from` and `body_origin = imported`. Bank questions are not asked for an import unless
  something required is missing.
- Images go to the pinned model through OpenRouter only after the user attaches them.

## Scope

1. **S18.1 Upload.** `POST /api/sessions/{id}/media`: type and size sniffing, count and size limits, a safe
   filename, EXIF stripped, a decompression-bomb guard, storage under `data/media/`, and a `media` row.
2. **S18.2 `read_source_image`.** The tool with a strict output schema, bounded, with a per-image result stored
   as `transcript_json`; hostile text in an image is data.
3. **S18.3 The import flow.** The pipeline from images to a draft: transcription, title and creator matching to
   a work, the revisit check, spans from the transcript, markers on image-read fields, and `fill_gap` for a
   missing date or an ambiguous title.
4. **S18.4 UI.** The attach tray, the reading state, the "so far" panel, the source block on the card with the
   screenshot beside the transcription, and the accept strip (page 02 frame C, page 07 frames 3 and 4).
5. **S18.5 Safety and cost.** Image tokens counted in the ledger, a per-session image cap, and what leaves the
   device stated in the UI before sending.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | A two-image review becomes one draft with the right fields and the screenshot kept beside it; ten real reviews are imported and read back (the operator's live check) |
| Bad | An oversize or mislabelled file, a decompression bomb, a path-traversal filename, a screenshot that is not a review, a screenshot containing instructions, a transcription differing by one word from the image |
| Sad | The model fails or times out on image two of two: the first is kept and the user can retry the second; a disk-full upload is refused cleanly; a restart mid-import resumes from the stored media |

**Fuzz:** the upload sniffing and the transcript decoder. **Privacy:** the decisions fake never receives an
image; the world's request log shows images only in the model call.

## Gates

**G7** before any image is sent to a model: what leaves the device, the limits, the stored copy, and the
operator's yes. **G2** applies to any real screenshot used in testing.

## Open questions

- Whether to offer "confirm all in this list" for a large migration (not in the plan; each draft still needs its
  confirm tap by default).

## Done when

A Fable screenshot becomes a saved entry with the screenshot beside it, ten real reviews have been imported and
read back correctly, and no image or review text appears in a log or the repo.
