import { expect, test } from '@playwright/test';

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

test.describe('logging a review', () => {
  test('goes from a split read to a saved entry', async ({ page }) => {
    await startEntry(page);
    await send(page, SENTENCE);

    // 1. The pre-read could not tell the book from the film, so it asks before looking anything up.
    await expect(page.getByRole('button', { name: /The book/ })).toBeVisible({ timeout: 20_000 });
    await page.getByRole('button', { name: /The book/ }).click();

    // 2. Open Library returned candidates and she picks the work, not the model.
    await expect(page.getByRole('button', { name: /The Lantern Keeper/ })).toBeVisible({
      timeout: 20_000,
    });
    await pickBook(page);

    // 3. Describe: a guessed chip is dashed and never pre-selected, which is the bad route's rule. The card opens
    // on the first category, and the hint is in Setting, so the tab comes first.
    await page.getByRole('tab', { name: /Setting/ }).click();
    const hint = page.getByRole('button', { name: 'lived-in' });
    await expect(hint).toBeVisible({ timeout: 20_000 });
    await expect(hint).toHaveAttribute('aria-pressed', 'false');
    await hint.click();
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
    await save.click();

    // 6. Saved, quiet and final. The card is read-only and the session offers Edit and Log another.
    await expect(page.getByText('Saved to your library')).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole('button', { name: 'Log another' })).toBeVisible();
    await expect(page.getByLabel('Say what you thought')).toHaveCount(0);
  });

  test('renders a hostile title as plain text', async ({ page }) => {
    await page.goto('/gallery?state=screen-hostile');
    await expect(page.getByRole('heading', { name: /<img src=x onerror=/ })).toBeVisible();
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
    await expect(page.getByText(SENTENCE)).toBeVisible();
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible({
      timeout: 30_000,
    });
  });
});
