import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { READING_URL, READING_WEBKIT_URL } from '../playwright.config';

/**
 * The reading screens: Home, Drafts, the Library, a saved entry with its versions, Deleted and Settings, against the
 * real service and a seeded invented library. One baseline and one axe pass per state, on a phone viewport, because
 * the mockups are frames of one (docs/TESTING.md 6).
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
  await expect(page).toHaveScreenshot('reading-home.png', {
    fullPage: true,
    mask: [page.locator(ages)],
  });
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
  await expect(page).toHaveScreenshot('reading-drafts.png', {
    fullPage: true,
    mask: [page.locator(ages)],
  });
});

test('resuming a draft returns to its card', async ({ page }) => {
  await openReading(page, '/drafts');
  await page.getByRole('button', { name: /Hollow Deep/ }).click();

  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await expect(page.getByLabel('Say what you thought')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Hollow Deep' })).toBeVisible();
});

test('the library groups by month, filters by medium, and sorts', async ({ page }) => {
  await openReading(page, '/library');

  await expect(page.getByRole('heading', { level: 1, name: 'Library' })).toBeVisible();
  await expect(page.getByText('October 2026')).toBeVisible();
  await expect(page.getByText('September 2026')).toBeVisible();
  // A saved entry without a finish date is not dropped: it is a group of its own at the end.
  await expect(page.getByText('No date')).toBeVisible();
  await noViolations(page);
  await expect(page).toHaveScreenshot('reading-library.png', { fullPage: true });

  // Filtering by medium is a real filter, not a scroll: only the films remain.
  await page.getByRole('button', { name: 'Film', exact: true }).click();
  await expect(page.getByRole('button', { name: /Bramble Hollow/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ })).toHaveCount(0);

  // Bad input with a way out: a medium with nothing in it says so and offers the way back.
  await page.getByRole('button', { name: 'Podcast', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'No podcast entries yet.' })).toBeVisible();
  await expect(page).toHaveScreenshot('reading-library-empty-filter.png', { fullPage: true });
  await page.getByRole('button', { name: 'Show everything' }).click();
  await expect(page.getByRole('button', { name: /The Lantern Keeper/ }).first()).toBeVisible();

  // The sort is the library's second order, and the best rating comes first. A reload puts the chip row back where it
  // started: choosing Podcast scrolled it, and a scrolled strip in a baseline is a baseline that differs by run.
  await page.reload();
  await expect(page.locator('.skel')).toHaveCount(0);
  await page.getByRole('button', { name: 'Sort: Newest first' }).click();
  await expect(page.getByRole('button', { name: 'Sort: Highest rated' })).toBeVisible();
  await expect(page.locator('.list .row').first()).toContainText('The Copper Room');
  await expect(page).toHaveScreenshot('reading-library-by-rating.png', { fullPage: true });
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
  await expect(page).toHaveScreenshot('reading-entry.png', { fullPage: true });
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
  await expect(page).toHaveScreenshot('reading-versions.png', {
    fullPage: true,
    mask: [page.locator(instants)],
  });

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
  await expect(page).toHaveScreenshot('reading-deleted.png', {
    fullPage: true,
    mask: [page.locator('.delrow .mm')],
  });

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
  await expect(page).toHaveScreenshot('reading-entry-deleted.png', {
    fullPage: true,
    mask: [page.locator('.deletednote p')],
  });

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
  await expect(page).toHaveScreenshot('reading-settings.png', { fullPage: true });

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

test('the first run teaches by example and a tap fills the composer', async ({ page }) => {
  // An empty library is answered for instead of a real one, because the seeded stack never is empty.
  await page.route('**/api/entries*', (route) =>
    route.fulfill({ status: 200, json: { items: [], next_cursor: null } }),
  );
  await page.route('**/api/sessions*', (route) =>
    route.fulfill({ status: 200, json: { items: [], next_cursor: null } }),
  );

  await openReading(page, '/');
  await expect(page.getByRole('heading', { name: 'Finished something?' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Drafts, 0 waiting' })).toBeVisible();
  await noViolations(page);
  await expect(page).toHaveScreenshot('reading-first-run.png', { fullPage: true });

  // Tapping an example fills the composer and sends nothing: the send button stays off until she sends it.
  const example = page.getByRole('button', { name: /finished the lantern keeper/ });
  await example.click();
  await expect(page.getByLabel('Say what you thought')).toHaveValue(/finished the lantern keeper/);
  await expect(page.getByRole('button', { name: 'Send' })).toBeEnabled();
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
  await expect(page).toHaveScreenshot('reading-library-failed.png', { fullPage: true });

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
  await expect(page).toHaveScreenshot('reading-entry-huge.png', { fullPage: false });
});

test('a malformed entry id shows a not-found state, not an empty page', async ({ page }) => {
  await openReading(page, '/entry/not-a-real-entry');
  await expect(page.getByRole('heading', { name: 'This entry is not here.' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Back to the library' })).toBeVisible();
  await noViolations(page);
});
