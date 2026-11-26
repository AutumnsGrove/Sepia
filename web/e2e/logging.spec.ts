import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import { shot } from './shot';

/**
 * The logging flow against the real service and the in-process world. The gallery proves what each state looks
 * like; this proves the states happen, in order, from a sentence she typed to an entry in her library.
 *
 * The dev stack is scripted for one invented review (`world.SampleMessage`), and the end-to-end stack runs the
 * split scenario: the pre-read cannot tell the book from the film, so every card on mockup 01 is asked.
 */

const SENTENCE =
  'finished the lantern keeper last night, 4.5, the ending landed but it was too long';

async function startEntry(page: import('@playwright/test').Page) {
  await page.goto('/new');
  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await expect(page.getByLabel('Say what you thought')).toBeVisible();
  // The composer is the textarea and Send: a paperclip with nothing behind it taught that the app was broken, and a
  // 44px control is under the baseline's pixel tolerance, so its absence is asserted here (issue #15).
  await expect(page.getByRole('button', { name: /attach/i })).toHaveCount(0);
}

/**
 * Pick the invented book, and answer the revisit card if the same work is already in the library. Every test logs
 * the same invented work, and the world carries one book, so the second flow on a stack is offered the choice.
 */
async function pickBook(page: import('@playwright/test').Page) {
  await page.getByRole('button', { name: /The Lantern Keeper/ }).click();
  const revisit = page.getByRole('button', { name: 'A new entry' });
  const setting = page.getByRole('tab', { name: /Setting/ });
  await expect(setting.or(revisit)).toBeVisible({ timeout: 20_000 });
  if (await revisit.isVisible()) await revisit.click();
}

/**
 * The questions are optional, and a card can exist before they are answered (the revisit flow starts one). Wait
 * until either the form is up or the card is ready, then skip the form: doing nothing must always be allowed.
 */
async function finishQuestions(page: import('@playwright/test').Page) {
  const skip = page.getByRole('button', { name: 'Skip', exact: true });
  const save = page.getByRole('button', { name: 'Save entry' });
  await expect
    .poll(async () => (await skip.isVisible()) || (await save.isEnabled()), { timeout: 20_000 })
    .toBe(true);
  if (await skip.isVisible()) await skip.click();
}

async function send(page: import('@playwright/test').Page, text: string) {
  await page.getByLabel('Say what you thought').fill(text);
  await page.getByRole('button', { name: 'Send' }).click();
}

/**
 * Answer or skip whatever the card is waiting on, until it is ready to save. Every flow on this stack logs the same
 * invented work, so a test that runs after another one can be asked a question the first flow already answered. Each
 * answer is given time to land before the next one: answering a question that has just been answered is refused, and
 * the refusal is drawn in the transcript, which is not a state a screenshot should carry.
 */
async function settleCard(page: import('@playwright/test').Page) {
  const save = page.getByRole('button', { name: 'Save entry' });
  const deadline = Date.now() + 25_000;
  let answers = 0;
  while (Date.now() < deadline) {
    if (await save.isEnabled().catch(() => false)) return;
    const answer = page.getByRole('button', { name: 'Done', exact: true }).first();
    const skip = page.getByRole('button', { name: 'Skip' }).last();
    if (answers < 3 && (await answer.isVisible().catch(() => false))) {
      await answer.click({ timeout: 3000 }).catch(() => undefined);
      answers += 1;
      await page.waitForTimeout(700);
    } else if (answers < 3 && (await skip.isVisible().catch(() => false))) {
      await skip.click({ timeout: 3000 }).catch(() => undefined);
      answers += 1;
      await page.waitForTimeout(700);
    } else {
      await page.waitForTimeout(250);
    }
  }
}

test.describe('logging a review', () => {
  test('goes from a split read to a saved entry', async ({ page }) => {
    await startEntry(page);
    await send(page, SENTENCE);

    // 1. The pre-read could not tell the book from the film, so it asks before looking anything up.
    await expect(page.getByRole('button', { name: /The book/ })).toBeVisible({ timeout: 20_000 });
    await page.getByRole('button', { name: /The book/ }).click();

    // 2. Open Library returned candidates and she picks the work, not the model. Each candidate draws its cover,
    // which the browser fetches from the app: the CSP allows img-src 'self' only, so a provider's own URL draws
    // nothing at all (issue #53). The image is asserted decoded, not merely present.
    await expect(page.getByRole('button', { name: /The Lantern Keeper/ })).toBeVisible({
      timeout: 20_000,
    });
    const cover = page.locator('.cand img').first();
    await expect(cover).toHaveAttribute('src', /^\/api\/thumb\?/);
    await expect
      .poll(async () => cover.evaluate((img: HTMLImageElement) => img.naturalWidth), {
        timeout: 10_000,
      })
      .toBeGreaterThan(0);
    await pickBook(page);

    // 3. Describe: a guessed chip is dashed and never pre-selected, which is the bad route's rule. The card opens
    // on the first category, and the hint is in Setting, so the tab comes first.
    await page.getByRole('tab', { name: /Setting/ }).click();
    const hint = page.getByRole('button', { name: 'lived-in' });
    await expect(hint).toBeVisible({ timeout: 20_000 });
    await expect(hint).toHaveAttribute('aria-pressed', 'false');
    await hint.click();
    // A swipe-back remounts the card while the interaction is still open. The tab and the picked chip come back
    // from the session draft, not from a component that no longer exists.
    await page.goBack();
    await page.goForward();
    await expect(page.getByRole('tab', { name: /Setting/ })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    await expect(page.getByRole('button', { name: 'lived-in' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await page.getByRole('button', { name: 'Done', exact: true }).click();

    // 4. The single questions, skipped: skipping is always allowed and the entry still saves. The world carries
    // one book, so a second flow on the same stack answers a revisit card first, and that counts toward the four
    // question rounds; the card is then built without the form. Either way, the entry saves.
    await finishQuestions(page);

    // 5. The card, built from her words.
    const save = page.getByRole('button', { name: 'Save entry' });
    await expect(save).toBeEnabled({ timeout: 20_000 });
    await expect(page.getByRole('button', { name: /Edit your words/ })).toBeVisible();
    await expect(page.getByText('Left out (1)')).toBeVisible();

    // A Describe pick is in the entry's answers, not silently dropped: the client's shape is the one the agent
    // decodes. This is the regression test for the old `{tags}` payload.
    await save.click();

    // 6. Saved, quiet and final. The card is read-only and the session offers Edit and Log another.
    await expect(page.getByText('Saved to your library')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole('button', { name: 'Log another' })).toBeVisible();
    await expect(page.getByLabel('Say what you thought')).toHaveCount(0);

    // Poll rather than read once: the saved ack and the list can cross, and WebKit is where they did.
    await expect
      .poll(
        async () => {
          const list = await page.request.get('/api/entries?limit=100');
          const library = (await list.json()) as {
            items: { answers: { category?: string; values: string[] }[] }[];
          };
          // Any entry with that answer, not items[0]: both browser projects log on this stack on the same day, and the
          // list's order breaks that tie on a generated id, so a page of one could be the other project's entry.
          return (
            library.items
              .flatMap((item) => item.answers ?? [])
              .find((answer) => answer.category === 'setting')?.values ?? []
          );
        },
        { timeout: 10_000 },
      )
      .toContain('lived_in');
  });

  test('the why sheet opens as a dialog from the card and from an activity row', async ({
    page,
  }) => {
    await startEntry(page);
    await send(page, SENTENCE);
    await page.getByRole('button', { name: /The book/ }).click();
    await pickBook(page);
    await page.getByRole('tab', { name: /Setting/ }).click();
    await page.getByRole('button', { name: 'Done', exact: true }).click();
    await finishQuestions(page);
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeEnabled({
      timeout: 20_000,
    });

    // The card's own trigger, then the activity row's, both open the same modal sheet. The row's "why" used to be
    // 26x18px and opened a block below the fold, so it looked dead.
    await page.getByRole('button', { name: 'How this was read' }).click();
    const sheet = page.getByRole('dialog', { name: 'How this was read' });
    await expect(sheet).toBeVisible();
    const results = await new AxeBuilder({ page }).analyze();
    expect(results.violations).toEqual([]);
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();

    const why = page.locator('.act .why').first();
    const box = await why.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    await why.click();
    await expect(sheet).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
  });

  test('renders a hostile title as plain text', async ({ page }) => {
    await page.goto('/gallery?state=screen-hostile');
    // The top bar and the card both carry the title, which is the point: it is text in every place it appears.
    const heading = page.getByRole('heading', { name: /<img src=x onerror=/ });
    await expect(heading).toHaveCount(2);
    await expect(heading.first()).toBeVisible();
    // The title is text, not markup: no image was created and no script ran.
    await expect(page.locator('article img')).toHaveCount(0);
    expect(
      await page.evaluate(() => (window as unknown as { __xss?: number }).__xss),
    ).toBeUndefined();
  });

  test('refuses a message over the limit in place', async ({ page }) => {
    await startEntry(page);
    await page.getByLabel('Say what you thought').fill('a'.repeat(8001));
    await expect(page.getByText('That message is too long. Split it into two.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Send' })).toBeDisabled();
  });

  test('keeps two devices on one session in step', async ({ page, context }) => {
    await startEntry(page);
    await send(page, SENTENCE);
    await page.getByRole('button', { name: /The book/ }).click();
    await pickBook(page);
    await page.getByRole('tab', { name: /Setting/ }).click();
    await page.getByRole('button', { name: 'Done', exact: true }).click();
    await finishQuestions(page);
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeEnabled({
      timeout: 20_000,
    });

    // A second device on the same session sees the card the hello carried, and every change after it.
    const other = await context.newPage();
    await other.goto(page.url());
    await expect(other.getByRole('button', { name: 'Save entry' })).toBeEnabled({
      timeout: 20_000,
    });

    await page.getByRole('button', { name: 'A quarter star more' }).click();
    await expect(other.getByRole('slider', { name: 'Your rating' })).toHaveAttribute(
      'aria-valuenow',
      '19',
      { timeout: 20_000 },
    );
  });

  test('keeps a revision in editing mode after a refresh', async ({ page }) => {
    await startEntry(page);
    await send(page, SENTENCE);
    await page.getByRole('button', { name: /The book/ }).click();
    await pickBook(page);
    await page.getByRole('tab', { name: /Setting/ }).click();
    await page.getByRole('button', { name: 'Done', exact: true }).click();
    await finishQuestions(page);
    const save = page.getByRole('button', { name: 'Save entry' });
    await expect(save).toBeEnabled({ timeout: 20_000 });
    await save.click();
    await expect(page.getByText('Saved to your library')).toBeVisible({ timeout: 20_000 });

    // Edit opens a revision. A refresh must not send reopen again to a session that already has its card, and
    // must still read the saved entry so the card stays in editing mode.
    await page.getByRole('button', { name: 'Edit' }).click();
    const banner = page.getByText(
      'You are editing a saved entry. Saving keeps the previous version.',
    );
    await expect(banner).toBeVisible({ timeout: 20_000 });
    await page.reload();
    await expect(banner).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole('button', { name: 'Save changes' })).toBeVisible();
    await expect(page.getByText('The card changed while you were editing.')).toHaveCount(0);

    // Saving the revision keeps the query in the URL. A refresh of the saved page must not ask to reopen a
    // finished session and leave a refusal above the card.
    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page.getByText('Saved to your library')).toBeVisible({ timeout: 20_000 });
    await page.reload();
    await expect(page.getByText('Saved to your library')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole('alert')).toHaveCount(0);
  });

  test('keeps a message typed offline and sends it when the connection returns', async ({
    page,
    context,
  }) => {
    await startEntry(page);
    await context.setOffline(true);
    await expect(page.getByText('Offline. Reconnecting.')).toBeVisible({ timeout: 30_000 });

    await send(page, SENTENCE);

    await context.setOffline(false);
    // A browser may hold the message in the outbox or deliver it over a socket it never noticed dying. Both are
    // allowed; what is not allowed is losing it or sending it twice, so the sentence appears exactly once and the
    // turn runs.
    await expect(page.getByRole('button', { name: /The book/ })).toBeVisible({ timeout: 45_000 });
    await expect(page.getByText(SENTENCE)).toHaveCount(1);
    await expect(page.getByText('Will send when you are back online')).toHaveCount(0);
  });

  // Mockup 04, frame 1e: stopping partway turns the Facts date into Stopped and adds one full-width How far fact. The
  // label above it used to say "How far" as well, so the section said the same word twice.
  test('stopping partway dates the card Stopped and asks how far she got', async ({ page }) => {
    await startEntry(page);
    await send(page, SENTENCE);
    await page.getByRole('button', { name: /The book/ }).click();
    await pickBook(page);
    // The Describe step is answered first and given time to land: an edit sent while the card is being refreshed is
    // refused as stale, and the refusal is drawn in the transcript.
    await settleCard(page);
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeEnabled({ timeout: 20_000 });

    // A stopped work needs words of her own, where a finished one only needs a rating. A card the model already filled
    // has some, and only her own typing may carry a how-far line she did not write, so which one it is is remembered.
    const writeWords = page.getByRole('button', { name: /Tap to write your own/ });
    const typed = await writeWords.isVisible();
    if (typed) {
      await writeWords.click();
      await page.getByLabel('Your words').fill('could not get into it after two episodes');
      await page.getByRole('article').getByRole('button', { name: 'Done', exact: true }).click();
      await expect(page.getByText('could not get into it after two episodes')).toBeVisible();
    }

    // The switch is on its own row under the work header, and there she stops.
    await page.getByRole('button', { name: "Didn't finish" }).click();
    await expect(page.getByRole('button', { name: "Didn't finish" })).toHaveAttribute(
      'aria-pressed',
      'true',
      { timeout: 20_000 },
    );

    // The date fact is called Stopped, never Finished, and the rating no longer nags.
    await expect(page.locator('.facts .fact .k', { hasText: 'Stopped' })).toHaveCount(1);
    await expect(page.locator('.facts .fact .k', { hasText: 'Finished' })).toHaveCount(0);
    await expect(page.getByText('Needs a rating')).toHaveCount(0);
    // One How far, and it is a fact in the grid rather than a label above one.
    await expect(page.locator('.facts .fact.wide .k')).toHaveText('How far');
    await expect(page.locator('p.lbl', { hasText: 'How far' })).toHaveCount(0);

    // A stopped work needs the day she stopped, because that is the date its month group is built from.
    await page.locator('.facts .fact', { hasText: 'Stopped' }).click();
    await page.getByLabel('Stopped on').fill('2026-11-03');
    await expect(page.locator('.facts .fact', { hasText: 'Stopped' })).toContainText('3 Nov');
    if (typed) {
      await page.getByLabel('How far you got').fill('about 100 pages in');
      await page.getByLabel('How far you got').blur();
    }

    // That shelf asks its own questions; they are skipped here because this test is about the card's facts.
    await settleCard(page);
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeEnabled({ timeout: 20_000 });

    const results = await new AxeBuilder({ page }).analyze();
    expect(results.violations).toEqual([]);
    await shot(page, 'logging-card-stopped.png');
  });

  test('a work she only means to read gets a card with nothing to score', async ({ page }) => {
    await startEntry(page);
    await send(page, 'want to read the lantern keeper, everyone says the ending is wild');

    // The pre-read may still ask which medium; the card is the book either way.
    const book = page.getByRole('button', { name: /The book/ });
    await expect
      .poll(
        async () =>
          (await book.isVisible()) ||
          (await page.getByRole('button', { name: /The Lantern Keeper/ }).isVisible()),
        {
          timeout: 20_000,
        },
      )
      .toBe(true);
    if (await book.isVisible()) await book.click();
    await page.getByRole('button', { name: /The Lantern Keeper/ }).click({ timeout: 20_000 });

    // pickBook waits for the Describe form, which a wish must not get. The revisit card can still come up, because
    // every flow on this stack logs the same invented work.
    const shelfControl = page.getByRole('button', { name: 'Wanted' });
    const revisit = page.getByRole('button', { name: 'A new entry' });
    await expect(shelfControl.or(revisit)).toBeVisible({ timeout: 20_000 });
    if (await revisit.isVisible()) await revisit.click();

    // The card knows it is a wish, so there is nothing to rate and nothing to answer: no Describe form, no stars.
    await expect(shelfControl).toHaveAttribute('aria-pressed', 'true', { timeout: 20_000 });
    await expect(page.getByRole('slider', { name: 'Your rating' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Save entry' })).toBeEnabled({ timeout: 20_000 });

    const results = await new AxeBuilder({ page }).analyze();
    expect(results.violations).toEqual([]);
  });
});

test.describe('when the model is down', () => {
  test('says so, keeps her words, and offers Try again', async ({ page }) => {
    // The second stack runs the same world with the chat fake failing every request.
    await page.goto('http://localhost:18894/new');
    await expect(page).toHaveURL(/\/s\/[^/]+$/);
    await send(page, SENTENCE);
    // The pre-read still works, so the medium card comes first; the model is only reached after her tap.
    await page.getByRole('button', { name: /The book/ }).click();

    await expect(
      page.getByText(
        'The model did not finish. Your words are kept; you can try again or fill in the card by hand.',
      ),
    ).toBeVisible({ timeout: 30_000 });
    // Her message is still in the transcript, and the way on is a retry, not a lost review.
    const notices = page.getByText(
      'The model did not finish. Your words are kept; you can try again or fill in the card by hand.',
    );
    await expect(notices).toHaveCount(1);
    await expect(page.getByText(SENTENCE)).toHaveCount(1);

    // The retry runs the turn again for the same message. The pre-read runs again too, so the medium card can come
    // back before the model is reached; answer it if it does. A second failure is a second notice, and her words
    // are still in the transcript once.
    await page.getByRole('button', { name: 'Try again' }).first().click();
    const book = page.getByRole('button', { name: /The book/ });
    await expect(book.or(notices.nth(1))).toBeVisible({ timeout: 20_000 });
    if (await book.isVisible()) await book.click();
    await expect(notices).toHaveCount(2, { timeout: 30_000 });
    await expect(page.getByText(SENTENCE)).toHaveCount(1);
  });
});
