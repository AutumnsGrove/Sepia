# G7: sending an image to a model, for the S01.6 vision spike (answered 2026-11-22)

Written 2026-11-22 by the agent starting S01.6. At the time of writing no image had been sent, no key had been used
and nothing had been spent. This file exists so Autumn could answer the gate with the whole shape of it in front of
her, and the answer is the section at the end.

**Status: answered 2026-11-22.** Autumn cleared the gate on four screenshots of her own reviews, and accepted the
unchecked retention setting. What was actually sent, and what it cost, is in the answer section; the questions below
are kept as they were put to her.

## 1. Where this is

S01.6 is the last spike in S01 and the only one that cannot be written up from shipped code. It asks one thing: when
the pinned model is handed a real screenshot of a Fable review, how much of it does it get right? Star icons, small
type, a review that runs across two images.

S18 (screenshot import, phase 3) is designed around the answer. If the model reads stars and dates reliably, the
import is mostly plumbing. If it does not, S18 needs her to grade and correct each imported draft, or a different
model, or both. That is why this runs before S18 rather than beside it.

What is already true: S00 and S02 to S14 and S21 are built and live (`r160.c3e3662`), the spike's own plumbing
(nova's embed, the pinned model, OpenRouter) is proven by production turns, and `docs/spikes/` does not exist yet.

## 2. What would leave the device, exactly

| What | Detail |
|---|---|
| The images | Five screenshots of her own Fable reviews, sent at the size they were taken. Nothing is cropped, redacted or downscaled unless she wants that |
| The prompt | About 600 characters of instructions: the output shape (title, creator, rating as shown, the date as shown, the review text, and a confidence for each field) and the rules ("transcribe, do not summarise"; "if a field is not visible, say so"). **No other review text, no other entry, no library data** |
| Where it goes | `https://openrouter.ai/api/v1/chat/completions`, model `deepseek/deepseek-v4.1-flash-1017`, authenticated with the key at `~/.pi/agent/auth.json` (`.openrouter.key`), read at runtime and never echoed, logged or committed. OpenRouter forwards the image and the prompt to whichever provider serves that model |
| What does not leave | The database, the rest of the library, her OpenRouter account details, anything from the sessions store, and any text from an image that is not in the image itself |

This is the same route S18 will take in production, so a spike answers the real question rather than a proxy one.

## 3. The limits

| What | Limit | Why that number |
|---|---|---|
| Images in this spike | **5** | S01.6 says about five; one of them is a review that spans two images, so five covers four titles and the across-two case |
| Image size | 10 MB each, the same as `STANDARDS.md` allows a message | An iPhone screenshot is 1 to 3 MB, so nothing needs resizing |
| Structure | `STANDARDS.md` allows 6 images a message; the model's context is 1,048,576 tokens, so five screenshots are nowhere near a context limit | No downscaling, no quality loss, which is the point: small type is what is being tested |
| EXIF | Stripped before sending, as S18.1 will do | A screenshot carries a device and a timestamp; neither is needed to read stars |
| Spend | Under **5 US cents** for the whole spike, and I will report the actual figure from the response's `usage` | The listing prices this model at $0.15 per million input tokens off-peak and $0.30 on-peak. Eight calls with a screenshot and a short answer each is under 3 cents of input and well under 1 cent of output |

**G1 is already answered** for the key at that path (she said so in the 2026-11-21 session). This gate is the one that
has to be answered before an image goes out, not before money does.

## 4. Where the copy lives, and what gets written down

- **The images:** `~/sepia-spike-shots/`, created with mode 0700, **outside the repository**. This is the path I
  proposed to Autumn; she has not confirmed it yet, so any path she prefers is fine.
- **The spike's code:** `spikes/s01.6-vision/` inside the repo, throwaway, importable by nothing, with a README
  saying what it did. It contains the request shape and the scoring sheet, never an image and never a transcript.
- **The finding:** `docs/spikes/01.6-vision.md`. It records what the model got right and wrong **by field and
  category**, never by quoting her review text: "rating: read 4 when five stars were filled, on 2 of 5 images",
  "date: read the release date rather than the watched date, on 1 of 5", "text: dropped the last line of image one on
  1 of 5, and dropped a word in the middle on 1 of 5". That is enough for S18's design and it is the most that can
  be said without her words entering the repo.
- **After the finding:** `rm -rf ~/sepia-spike-shots/`. The images are hers to keep wherever she likes; the spike's
  copy does not outlive the measurement.

## 5. What I verified for free, with no image sent and nothing spent

- **The pinned model takes images.** The live model listing
  (`https://openrouter.ai/api/v1/models`, public, no key) gives `deepseek/deepseek-v4.1-flash-1017` an
  `architecture.modality` of `text+image->text` with input modalities `[text, image]` and an output of `text`. That is
  the claim `PLAN.md` line 115 makes from its listing; it still holds today.
- **Its price and its shape of price.** $0.15/M prompt and $0.60/M completion, with a weekday override to $0.30/M and
  $1.20/M between 01:00 and 04:00 UTC and between 06:00 and 10:00 UTC. The spike's cost estimate above uses the
  higher of the two.
- **`DefaultModel` is that same id** (`internal/config/config.go:30`), so the spike tests the model production will
  actually use, not a stand-in.
- **`supported_parameters` includes `tools`, `structured_outputs` and `response_format`**, which is what S18.2's
  strict output schema needs.

## 6. What I asked Autumn for (answered below)

### 6.1 Two images, or five? (the gate itself)

She has said she has **no Fable screenshots**. So the accuracy question cannot be answered as S01.6 wrote it until
some exist. Three honest routes, and they are not exclusive:

| Route | What it proves | What it costs |
|---|---|---|
| **A. She takes five in Fable now**, so `~/sepia-spike-shots/` gets real screenshots before Fable is gone | The real question: does the model read *Fable's* stars and type | Ten minutes, and the screenshots are needed for the migration anyway |
| **B. Two phases:** run the plumbing now on a Sepia mockup render (no review content), then the accuracy pass on route A's images when they exist | That the request shape, the schema and the cost are right, without spending her privacy budget on a guess | Ten minutes now; the accuracy answer waits |
| **C. Synthetic only.** Render Fable-shaped screenshots locally | Nothing about Fable. It would test a UI we invented | Almost nothing, and it risks designing S18 on a fiction |

**Recommendation: B, then A.** The plumbing half costs under a cent and de-risks the accuracy half, so the five
screenshots she takes later are spent on a spike that already works. If she would rather do A straight away, that is
better still and I will take the five images in one run.

### 6.2 Her yes on the images themselves

Under **G7**: yes or no to images of her reviews being sent to OpenRouter and on to the model's provider. Under **G2**:
her acknowledgment that these are real reviews, that they stay outside the repo, and that only the findings above are
written down. If her OpenRouter account has a data-retention or training setting she wants off for this key, now is
the moment; I cannot see or change that setting, and it is worth knowing before an image of her own writing goes out.

## 7. What happens next (as written before the answer arrived)

1. She answers 6.1 and 6.2, and names a directory if she wants a different one than `~/sepia-spike-shots/`.
2. I create `spikes/s01.6-vision/` behind a `spikes/` that nothing imports, with the request and the scoring sheet.
3. Letter B: one run on a mockup render, then STOP for her five screenshots. Letter A: one run on all five.
4. I grade field by field, write `docs/spikes/01.6-vision.md` with the numbers and the cost, apply whatever it
   changes to `PLAN.md`, S01 and S18 in the same commit, and delete the spike's copies of her images.
5. S18 gets designed around what the model actually gets right.

## 8. Risks

- **A vision model can misread a rating confidently.** The measurement has to be per field against her own grading,
  never "it looked right". That is why the scoring sheet is part of the spike.
- **OpenRouter is a third party in the path.** The image and the prompt reach it and, depending on the account's
  retention setting, may be retained there. This gate is the right place to decide that, which is why it asks.
- **A screenshot can contain more than the review** (a name in a corner, another app's notification). She picks the
  images, and cropping them to the review is fine if she prefers.
- **The repo is public.** No image, no transcript and no quoted review text goes into `docs/spikes/`, the build log, a
  fixture or a test; only the findings in the shape section 4 describes.

## G7 answered, 2026-11-22

Autumn answered all three questions in one message, and the gate is cleared on these terms:

1. **She has the screenshots.** Four of them, in `~/Downloads`, the four newest files: `IMG_0293.PNG` to
   `IMG_0296.PNG`, 1206x2622 PNGs of 408 KB to 524 KB each. So route **A** runs, read against real Fable reviews
   rather than a mockup render; route B's plumbing-only pass is not needed first.
2. **The images may be sent.** G7 is cleared: image bytes and the short instructions go to OpenRouter and on to the
   model's provider.
3. **Retention: she said go ahead anyway.** The account's retention or training setting was not checked first, and she
   accepted that. It is recorded here so the finding states the assumption instead of implying a guarantee.

What changes below: "five images" becomes four (she has four; the two-image review case is covered if one of them spans
two, and the scoring sheet says so rather than inventing a fifth).

**A correction to the paragraph above, added the same day.** The four screenshots are all the *Finished* list, not an
opened review: they show title, creator, a star rating and a date per row, and no review body at all. So they answer
the part of S01.6 about star icons and small type, and they do **not** answer the part about a review's text or about
one review across two images. The finding says which half is measured; the other half needs four screenshots of an
opened review.

**What it cost, the figure section 3 promised.** Four calls, run on 2026-11-22 from the spike harness: **1,340 prompt
tokens each**, 736 to 844 completion tokens each, **2.8 to 6.5 s** per call, and **US$0.0054 in total** at the
listing's weekday peak price, which is about **$0.0014 per screenshot**. The gate's estimate was under 5 cents; the run
came in at a fifth of the ceiling. The per-call numbers and the grading are in
[`../spikes/01.6-vision.md`](../spikes/01.6-vision.md).

What does not change: the copy lives in `~/sepia-spike-shots/` with mode 0700, outside the repository; the finding
records mistakes by field and category and never quotes her review text; the spike's own copies are deleted when the
measurement is written up; and the harness and the scoring sheet live in the finding itself
([`../spikes/01.6-vision.md`](../spikes/01.6-vision.md)), because the architecture test would reject a `spikes/`
package that names a provider endpoint, so the runnable part stayed outside the module at
`~/sepia-spike-shots/spike/` and is not in the repository at all.
