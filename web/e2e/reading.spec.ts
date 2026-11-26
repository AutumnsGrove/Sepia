import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { READING_URL, READING_WEBKIT_URL } from '../playwright.config';
import { shot } from './shot';

/**
 * The reading screens: Home, Drafts, the Library, a saved entry with its versions, Deleted and Settings, against the
 * real service and a seeded invented library. One axe pass per state on a phone viewport, because the mockups are
 * frames of one (docs/TESTING.md 6), and a pixel baseline when `make screenshots` asks for one.
 *
 * The file runs serially: two of its states change the library (delete, restore), and a screenshot taken while
 * another test is mid-delete is a flaky screenshot. It is the only file on this stack, so serial costs nothing else.
 */
test.describe.configure({ mode: 'serial' });

/**
 * Each browser project has its own seeded stack, because this file deletes and restores entries: two projects on one
 * library would race each other's assertions and each other's screenshots.
 */
function stack(): string {
  return test.info().project.name === 'webkit-phone' ? READING_WEBKIT_URL : READING_URL;
}

// The stack is seeded when it starts, so an age ("2 min ago") and a version's instant differ on every run. They are
// masked rather than frozen: the layout is the claim, and the age is asserted as text below.
const ages = '.age';
const instants = '.when';

async function openReading(page: Page, path: string) {
  await page.goto(stack() + path);
  await expect(page.locator('.skel')).toHaveCount(0, { timeout: 15_000 });
}

async function noViolations(page: Page) {
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
}

/** A row's id, read from the API so no test carries an identifier that a reseed would change. */
async function entryID(page: Page, title: string, deleted = false): Promise<string> {
  const resp = await page.request.get(
    `${stack()}/api/entries?limit=100${deleted ? '&deleted=true' : ''}`,
  );
  const body = (await resp.json()) as { items: { id: string; work: { title: string } }[] };
  // The list is newest first, so the first match is the one the reading screens show at the top of the library.
  const found = body.items.filter((entry) => entry.work.title === title);
  if (found.length === 0) throw new Error(`no entry called ${title}`);
  return found[0].id;
}

test('home is a launcher, with the drafts first and the composer at the thumb', async ({
  page,
}) => {
  await openReading(page, '/');

  await expect(page.getByRole('heading', { level: 1, name: 'Sepia' })).toBeVisible();
  await expect(page.getByText('Waiting for you')).toBeVisible();
  // Hollow Deep is both a waiting draft and a saved game, so the draft's own name carries its reason.
  await expect(page.getByRole('button', { name: 'Hollow Deep Needs a rating' })).toBeVisible();
  await expect(page.getByRole('button', { name: /Grey Harbour/ })).toContainText(
    'Needs your words',
  );
  await expect(page.getByText('Recent')).toBeVisible();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ }).first()).toBeVisible();
  await expect(page.getByLabel('Say what you thought')).toBeVisible();
  // The drafts badge is a count and its name says what it counts.
  await expect(page.getByRole('link', { name: 'Drafts, 2 waiting' })).toBeVisible();

  await noViolations(page);
  await shot(page, 'reading-home.png', { mask: [page.locator(ages)] });
});

test('the drafts list says what each draft is waiting for', async ({ page }) => {
  await openReading(page, '/drafts');

  await expect(page.getByRole('heading', { level: 1, name: 'Drafts' })).toBeVisible();
  // Two drafts, each saying what it is waiting for. The order within one instant is by id, so the names are what the
  // test holds on to rather than a position.
  await expect(page.locator('.list .row')).toHaveCount(2);
  await expect(page.getByRole('button', { name: 'Hollow Deep Needs a rating' })).toBeVisible();
  await expect(page.getByRole('button', { name: /Grey Harbour/ })).toContainText(
    'Needs your words',
  );

  await noViolations(page);
  await shot(page, 'reading-drafts.png', { mask: [page.locator(ages)] });
});

test('resuming a draft returns to its card', async ({ page }) => {
  await openReading(page, '/drafts');
  await page.getByRole('button', { name: /Hollow Deep/ }).click();

  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await expect(page.getByLabel('Say what you thought')).toBeVisible();
  // The screen titles the card in its top bar and the card names the work under it, so both are headings.
  await expect(page.getByRole('heading', { level: 1, name: 'Hollow Deep' })).toBeVisible();
  await expect(page.getByRole('heading', { level: 2, name: 'Hollow Deep' })).toBeVisible();
});

test('the library groups by month, filters by medium, and sorts', async ({ page }) => {
  await openReading(page, '/library');

  await expect(page.getByRole('heading', { level: 1, name: 'Library' })).toBeVisible();
  await expect(page.getByText('October 2026')).toBeVisible();
  await expect(page.getByText('September 2026')).toBeVisible();
  // A saved entry without a finish date is not dropped: it is a group of its own at the end.
  await expect(page.getByText('No date')).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-library.png');

  // Filtering by medium is a real filter, not a scroll: only the films remain.
  await page.getByRole('button', { name: 'Film', exact: true }).click();
  await expect(page.getByRole('button', { name: /Bramble Hollow/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ })).toHaveCount(0);

  // Bad input with a way out: a medium with nothing in it says so and offers the way back.
  await page.getByRole('button', { name: 'Podcast', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'No podcast entries you have reviewed yet.' }),
  ).toBeVisible();
  await shot(page, 'reading-library-empty-filter.png');
  await page.getByRole('button', { name: 'Show everything' }).click();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ }).first()).toBeVisible();

  // The sort is the library's second order, and the best rating comes first. A reload puts the chip row back where it
  // started: choosing Podcast scrolled it, and a scrolled strip in a baseline is a baseline that differs by run.
  await page.reload();
  await expect(page.locator('.skel')).toHaveCount(0);
  await page.getByRole('button', { name: 'Sort: Newest first' }).click();
  await expect(page.getByRole('button', { name: 'Sort: Highest rated' })).toBeVisible();
  await expect(page.locator('.list .row').first()).toContainText('The Copper Room');
  await shot(page, 'reading-library-by-rating.png');
});

test('searching her own words finds the sentence, marks the match, and clears back', async ({
  page,
}) => {
  await openReading(page, '/library');

  // A word from a review, not a title: search reads what she wrote.
  const box = page.getByLabel('Search titles and your words');
  await box.fill('slow');
  await expect(page.getByText(/mentions? “slow”/)).toBeVisible();
  await expect(page.getByRole('button', { name: /Grey Harbour/ })).toBeVisible();
  // The matched word is marked in the sentence the server found.
  await expect(page.locator('.sn mark.m').first()).toHaveText('slow');
  await noViolations(page);
  await shot(page, 'reading-library-search.png');

  // Sad: words nothing in the library holds say so plainly, with a way back.
  await box.fill('zzzznothing');
  await expect(page.getByRole('heading', { name: /Nothing mentions/ })).toBeVisible();
  await shot(page, 'reading-library-search-empty.png');
  await page.getByRole('button', { name: 'Back to the library' }).click();
  await expect(page.getByText('October 2026')).toBeVisible();

  // Bad: FTS syntax is asked as words rather than obeyed, so it answers nothing instead of erroring.
  await box.fill('" OR 1=1 --');
  await expect(page.getByRole('heading', { name: /Nothing mentions/ })).toBeVisible();
  await page.getByRole('button', { name: 'Clear search' }).click();
  await expect(page.getByText('October 2026')).toBeVisible();
});

test('the library chat answers from her own words, and does not move an entry', async ({
  page,
}) => {
  await openReading(page, '/library');

  // The Ask button is the only door to the chat, so it is checked from the screen that owns it.
  await page.getByRole('link', { name: 'Ask your library' }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'Ask your library' })).toBeVisible();
  await expect(
    page.getByText('Ask about anything you have logged, or change an entry.'),
  ).toBeVisible();
  // The three ways in the mockup offers (04, frame 3).
  for (const example of [
    'which films did i give 4.5 or more?',
    'what did i read in march 2024?',
    'make hades 2 a 4.5',
  ]) {
    await expect(page.getByRole('button', { name: example })).toBeVisible();
  }
  await noViolations(page);
  await shot(page, 'reading-chat-start.png');

  // Asking is what creates the chat, so asking from the composer is the whole flow: a session, its first question, and
  // an answer built from the library the app already has.
  await page.getByLabel('Ask about your library').fill('slow');
  await page.getByRole('button', { name: 'Send' }).click();
  await expect(page).toHaveURL(/\/chat\/[^/]+$/);
  await expect(page.getByText('Here is what I found.')).toBeVisible({ timeout: 20_000 });

  // The answer's row is a tap to the entry it names, and it is drawn from the entry rather than typed by the model.
  const row = page.getByRole('button', { name: /Grey Harbour/ });
  await expect(row).toBeVisible();
  await expect(row).toContainText('TV');
  await noViolations(page);
  await shot(page, 'reading-chat-answer.png');

  // Sad: nothing matching says so plainly, and the chat stays open for the next question.
  await page.getByLabel('Ask about your library').fill('zzzznothing');
  await page.getByRole('button', { name: 'Send' }).click();
  await expect(page.getByText('Nothing in her library matches that.')).toHaveCount(0);

  await row.click();
  await expect(page.getByRole('heading', { level: 2, name: 'Grey Harbour' })).toBeVisible();

  // Back on the start screen, the chat she just had is listed under its own question: that list is what makes a past
  // answer findable again, and it is read from the sessions route rather than remembered in the page.
  await openReading(page, '/chat');
  await expect(page.getByText('Recent chats')).toBeVisible();
  await expect(page.getByRole('button', { name: /^slow/ }).first()).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-chat-recent.png');
});

test('three shelves, each a list of its own, with its own order and row', async ({ page }) => {
  await openReading(page, '/library');

  // Finished opens first. A dropped work and a wanted work are not in it: they are not reviews.
  await expect(page.getByRole('tab', { name: /^Finished/ })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(page.getByText('October 2026')).toBeVisible();
  await expect(page.getByRole('button', { name: /A Cold Inheritance/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /The Quarry Year/ })).toHaveCount(0);
  // Every tab carries a number, and the number is the shelf's total, not the filter's: the chip row narrows the list
  // below and leaves the counts alone.
  const finishedTab = page.getByRole('tab', { name: /^Finished/ });
  const before = await finishedTab.locator('.tc').textContent();
  expect(Number(before)).toBeGreaterThan(0);
  await page.getByRole('button', { name: 'Film', exact: true }).click();
  await expect(finishedTab.locator('.tc')).toHaveText(before as string);
  await page.getByRole('button', { name: 'All', exact: true }).click();
  await noViolations(page);
  await shot(page, 'reading-library.png');

  // Didn't finish: grouped by the month she stopped, no stars to give, the order stated rather than offered.
  await page.getByRole('tab', { name: /^Didn't finish/ }).click();
  const dropped = page.getByRole('button', { name: /A Cold Inheritance/ });
  await expect(dropped).toBeVisible();
  await expect(dropped).toContainText('stopped 12 Nov');
  await expect(dropped).toContainText('two episodes and the first ten minutes of the third');
  await expect(dropped).not.toContainText('★');
  await expect(page.getByText('November 2026')).toBeVisible();
  await expect(page.getByRole('button', { name: /^Sort:/ })).toHaveCount(0);
  await expect(page.getByLabel('Order: Newest first')).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-library-stopped.png');

  // Wanted: one flat list, the creator where the date would be, ordered by when she added it.
  await page.getByRole('tab', { name: /^Wanted/ }).click();
  const wanted = page.getByRole('button', { name: /The Quarry Year/ });
  await expect(wanted).toBeVisible();
  await expect(wanted).toContainText('Book · S. Adeyemi');
  await expect(wanted).not.toContainText('★');
  // No month heading: a wish has no month she finished it.
  await expect(page.getByText('November 2026')).toHaveCount(0);
  await expect(page.getByRole('button', { name: /^Sort:/ })).toHaveCount(0);
  await expect(page.getByLabel('Order: Recently added')).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-library-wanted.png');

  // The entry it opens says the same thing, and offers no rating for something she has not read: the one date is when
  // she added it, and the one action is to log it (mockup 04, frame 1b).
  await wanted.click();
  await expect(page.getByRole('heading', { level: 2, name: 'The Quarry Year' })).toBeVisible();
  await expect(page.locator('.shelfbadge')).toHaveText('Wanted');
  await expect(page.locator('.ratingrow')).toHaveCount(0);
  await expect(page.locator('.stamp')).toContainText('Added');
  await expect(page.getByRole('button', { name: 'Log a review' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit' })).toHaveCount(0);
  await noViolations(page);
  await shot(page, 'reading-entry-wanted.png');

  // A stopped entry says Stopped, never Finished, and her how-far line is in her words, in quotes (frame 1c).
  await page.goto(stack() + '/library');
  await page.getByRole('tab', { name: /^Didn't finish/ }).click();
  await page.getByRole('button', { name: /A Cold Inheritance/ }).click();
  await expect(page.getByRole('heading', { level: 2, name: 'A Cold Inheritance' })).toBeVisible();
  await expect(page.locator('.shelfbadge')).toHaveText("Didn't finish");
  await expect(page.locator('.stamp')).toContainText('Stopped Thu 12 Nov');
  await expect(page.locator('.progress')).toContainText(
    '“two episodes and the first ten minutes of the third”',
  );
  await expect(page.locator('.ratingrow')).toHaveCount(0);
  await noViolations(page);
  await shot(page, 'reading-entry-stopped.png');
});

test('each empty shelf says what it is for, and offers the one thing that fills it', async ({
  page,
}) => {
  await openReading(page, '/library?');
  // A medium with nothing on a shelf: the shelf's own sentence, and the way back to everything.
  await page.getByRole('tab', { name: /^Wanted/ }).click();
  await page.getByRole('button', { name: 'Podcast', exact: true }).click();
  await expect(
    page.getByRole('heading', { name: 'No podcast entries on your wanted list yet.' }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Show everything' }).click();
  await expect(page.getByRole('button', { name: /The Quarry Year/ })).toBeVisible();
});

test('a saved entry shows her words, the same work and its details', async ({ page }) => {
  await openReading(page, `/entry/${await entryID(page, 'The Lantern Keeper')}`);

  await expect(page.getByRole('heading', { level: 2, name: 'The Lantern Keeper' })).toBeVisible();
  await expect(page.getByText('The narrator makes the quiet parts work.')).toBeVisible();
  await expect(page.getByText('memorable')).toBeVisible();
  await expect(page.getByText('Also on this work (1)')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Log again' })).toBeVisible();
  await expect(page.getByText('Details')).toBeVisible();

  await noViolations(page);
  await shot(page, 'reading-entry.png');
});

test('an entry with history shows its versions and restores one as a draft', async ({ page }) => {
  const id = await entryID(page, 'Bramble Hollow');
  await openReading(page, `/entry/${id}`);
  await page.getByRole('button', { name: '2 earlier versions' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Earlier versions' })).toBeVisible();
  await expect(page.getByText('Version 3')).toBeVisible();
  await expect(page.getByText('Version 1')).toBeVisible();
  await expect(page.getByText('First saved.')).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-versions.png', { mask: [page.locator(instants)] });

  // Restore as draft opens the old version in a new session, with the old words on the card.
  await page.getByRole('button', { name: 'Restore as draft' }).last().click();
  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await expect(page.getByText('loved it honestly, easy 4.5')).toBeVisible({ timeout: 15_000 });
});

test('deleting is inline, keeps by default, and Restore brings the entry back', async ({
  page,
}) => {
  await openReading(page, `/entry/${await entryID(page, 'Night Signal')}`);

  // Named exactly: "A quarter star more" is also a button called more.
  await page.getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  const confirm = page.locator('.confirmbox');
  await expect(confirm).toContainText('Nothing is erased');
  // Keep it is the default: it is the primary action beside the danger.
  await expect(confirm.getByRole('button', { name: 'Keep it' })).toHaveClass(/primary/);
  await confirm.getByRole('button', { name: 'Keep it' }).click();
  await expect(confirm).toHaveCount(0);
  await expect(page.getByRole('heading', { level: 2, name: 'Night Signal' })).toBeVisible();

  // Named exactly: "A quarter star more" is also a button called more.
  await page.getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  await page.locator('.confirmbox').getByRole('button', { name: 'Delete' }).click();

  // It is out of the library and in the Deleted list, where the row says when it happened.
  await expect(page).toHaveURL(/\/deleted$/);
  const row = page.locator('.delrow').filter({ hasText: 'Night Signal' });
  await expect(row).toBeVisible();
  await expect(row).toContainText('Deleted');
  await noViolations(page);
  await shot(page, 'reading-deleted.png', { mask: [page.locator('.delrow .mm')] });

  await row.getByRole('button', { name: 'Restore' }).click();
  await expect(page.locator('.delrow').filter({ hasText: 'Night Signal' })).toHaveCount(0);
  await openReading(page, '/library');
  await expect(page.getByRole('button', { name: /Night Signal/ })).toBeVisible();
});

test('a deleted entry opened by a stale link says so, and offers Restore', async ({ page }) => {
  const id = await entryID(page, 'The Longest Winter', true);
  await openReading(page, `/entry/${id}`);

  const note = page.locator('.deletednote');
  await expect(note).toContainText('It is out of your library');
  await expect(note.getByRole('button', { name: 'Restore' })).toBeVisible();
  await noViolations(page);
  await shot(page, 'reading-entry-deleted.png', { mask: [page.locator('.deletednote p')] });

  await note.getByRole('button', { name: 'Restore' }).click();
  await expect(page.locator('.deletednote')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Edit' })).toBeVisible();
});

test('the pre-read switch is real: it is stored and it stays', async ({ page }) => {
  await openReading(page, '/settings');

  await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible();
  const toggle = page.getByRole('switch', { name: 'Quick pre-read' });
  await expect(toggle).toHaveAttribute('aria-checked', 'true');
  await noViolations(page);
  await shot(page, 'reading-settings.png');

  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-checked', 'false');
  await page.reload();
  await expect(page.getByRole('switch', { name: 'Quick pre-read' })).toHaveAttribute(
    'aria-checked',
    'false',
  );
  // Back on, so the next run of this file starts where it did.
  await page.getByRole('switch', { name: 'Quick pre-read' }).click();
  await expect(page.getByRole('switch', { name: 'Quick pre-read' })).toHaveAttribute(
    'aria-checked',
    'true',
  );
});

test('updates reports a missing helper rather than pretending to update', async ({ page }) => {
  await openReading(page, '/settings/updates');

  await expect(page.getByRole('heading', { level: 1, name: 'Updates' })).toBeVisible();
  // This stack runs the real binary with no signal directory, which is the code's own "bare" deployment: the panel
  // must say so and where to look, not show a button that could never work.
  await expect(page.getByText('Updates are not working on this server.')).toBeVisible();
  await expect(page.getByText('systemctl status sepia-update.path')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Check for updates' })).toHaveCount(0);

  await noViolations(page);
  await shot(page, 'reading-updates-no-helper.png');
});

test('services names every dependency and what an outage costs', async ({ page }) => {
  await openReading(page, '/settings/services');

  await expect(page.getByRole('heading', { level: 1, name: 'Services' })).toBeVisible();
  for (const name of ['Main model', 'Decisions', 'TMDB', 'Open Library', 'Polaris', 'Database']) {
    await expect(page.getByText(name, { exact: true })).toBeVisible();
  }
  // The seeded stack has no keys and no update helper, so the state is words rather than a colour.
  await expect(page.getByText('Not set up').first()).toBeVisible();
  await expect(page.getByText('Ready').first()).toBeVisible();
  await expect(page.getByText('Web lookups are off. Everything else still works')).toBeVisible();

  await noViolations(page);
  await shot(page, 'reading-services.png');
});

test('the first run asks the question and offers no words of its own', async ({ page }) => {
  // An empty library is answered for instead of a real one, because the seeded stack never is empty.
  await page.route('**/api/entries*', (route) =>
    route.fulfill({ status: 200, json: { items: [], next_cursor: null } }),
  );
  await page.route('**/api/sessions*', (route) =>
    route.fulfill({ status: 200, json: { items: [], next_cursor: null } }),
  );

  await openReading(page, '/');
  await expect(page.getByRole('heading', { name: 'Finished something?' })).toBeVisible();
  await expect(page.getByText(/Say what you thought, the way you would out loud/)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Drafts, 0 waiting' })).toBeVisible();
  // Nothing invented to tap: a suggested review would be the app putting words in her mouth, so the empty state is
  // the heading, the explanation and an empty composer, and Send stays off until she has typed something.
  await expect(page.getByLabel('Say what you thought')).toHaveValue('');
  await expect(page.getByRole('button', { name: 'Send' })).toBeDisabled();
  // The composer is the textarea and Send. The 1% pixel tolerance cannot see a 44px control disappear, so the
  // absence of the old disabled paperclip is asserted rather than left to a baseline (issue #15).
  await expect(page.getByRole('button', { name: /attach/i })).toHaveCount(0);
  await noViolations(page);
  await shot(page, 'reading-first-run.png');
});

test('a slow library shows skeleton rows rather than a spinner in the middle', async ({ page }) => {
  await page.route('**/api/entries*', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 1500));
    await route.continue();
  });

  await page.goto(stack() + '/library');
  await expect(page.locator('.skel')).toHaveCount(8);
  await expect(page.getByRole('status').first()).toHaveAttribute('aria-live', 'polite');
  await expect(page.locator('.skel')).toHaveCount(0, { timeout: 15_000 });
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ }).first()).toBeVisible();
});

test('a failed list says so and offers a retry that works', async ({ page }) => {
  let fail = true;
  await page.route('**/api/entries*', async (route) => {
    if (!fail) return route.continue();
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'internal', message: 'Something went wrong. Try again.' }),
    });
  });

  await page.goto(stack() + '/library');
  const alert = page.getByRole('alert');
  await expect(alert).toBeVisible();
  await expect(alert).toContainText('Try again');
  await shot(page, 'reading-library-failed.png');

  fail = false;
  await alert.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ }).first()).toBeVisible();
});

test('a very long title and a very long review do not break the layout', async ({ page }) => {
  const id = await entryID(page, 'The Salt Road');
  const resp = await page.request.get(`${stack()}/api/entries/${id}`);
  const entry = (await resp.json()) as Record<string, unknown> & {
    work: Record<string, unknown>;
  };
  const hostile = {
    ...entry,
    body: 'A sentence that keeps going. '.repeat(300),
    work: { ...entry.work, title: 'The'.repeat(100) },
  };
  await page.route(`**/api/entries/${id}`, (route) =>
    route.fulfill({ status: 200, json: hostile }),
  );

  await openReading(page, `/entry/${id}`);
  await expect(page.getByRole('heading', { level: 2 })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
  await shot(page, 'reading-entry-huge.png', { fullPage: false });
});

test('a malformed entry id shows a not-found state, not an empty page', async ({ page }) => {
  await openReading(page, '/entry/not-a-real-entry');
  await expect(page.getByRole('heading', { name: 'This entry is not here.' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Back to the library' })).toBeVisible();
  await noViolations(page);
});

test('a sentence typed on Home is the first message of the session it starts', async ({ page }) => {
  // The one path from the composer to a session: Home's send stashes the sentence in memory, `/new` makes the session
  // and sends it as the first message. Nothing else covers that handoff, and the old first-run test only read the
  // composer's value back, so sending the wrong string would have passed it. Last in the file, because it leaves a
  // draft in the library.
  const sentence =
    'finished the lantern keeper last night, 4.5, the ending landed but it was too long';

  await openReading(page, '/');
  await page.getByLabel('Say what you thought').fill(sentence);
  await page.getByRole('button', { name: 'Send' }).click();

  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await expect(page.locator('.you p').first()).toHaveText(sentence);
});
