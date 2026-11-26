import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { shot } from './shot';

/**
 * The gallery is the unit of visual regression (S10.3): one axe scan per component state on a phone viewport, and a
 * pixel baseline when asked for (`make screenshots`). A pixel diff is evidence, not proof, so look at the diff before
 * updating a baseline; the behavioural assertions live in the Go and Vitest suites.
 */

const states = [
  'buttons',
  'pills',
  'chips',
  'switch',
  'tabs',
  'status',
  'banner',
  'errbox',
  'covers',
  'cloud',
  'list',
  'rating',
  'ix-medium',
  'ix-candidate',
  'ix-describe',
  'ix-form',
  'ix-revisit',
  'ix-same-work',
  'ix-gap',
  'ix-lookup',
  'screen-split',
  'screen-candidates',
  'screen-describe',
  'screen-draft',
  'screen-repeat-pre-read',
  'screen-saved',
  'screen-unverified',
  'screen-fidelity',
  'screen-editing',
  'screen-missing-rating',
  'screen-stopped',
  'screen-chat',
  'screen-chat-confirm',
  'screen-offline',
  'screen-caught-up',
  'screen-working',
  'screen-queued',
  'screen-error',
  'screen-preread-down',
  'screen-lookup-failed',
  'screen-hostile',
  'screen-stale',
  'upd-loading',
  'upd-no-helper',
  'upd-helper-quiet',
  'upd-checking',
  'upd-up-to-date',
  'upd-check-failed',
  'upd-available',
  'upd-blocked',
  'upd-updating',
  'upd-slow',
  'upd-done',
  'upd-failed',
  'upd-rollback-failed',
];

test.describe('the gallery', () => {
  // Not in the `states` list: this one has no section to screenshot, it has a fallback to find. It is the probe for
  // issue #40, where a render error replaced the whole app with SvelteKit's bare 500 page.
  test('a screen that throws keeps the app and offers a way out', async ({ page }) => {
    await page.goto('/gallery?state=boundary');
    await expect(page.getByRole('heading', { name: 'This screen did not load' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Back' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Reload' })).toBeVisible();
    // The fallback is a screen inside the app, not the whole document.
    await expect(page.locator('main')).toHaveCount(1);
  });

  test('a path that does not exist says so, and points home', async ({ page }) => {
    await page.goto('/no-such-screen');
    await expect(page.getByRole('heading', { name: 'There is nothing here' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Home' })).toBeVisible();
  });

  test('lists every state', async ({ page }) => {
    await page.goto('/gallery');
    await expect(page.getByRole('heading', { level: 1, name: 'Gallery' })).toBeVisible();
    for (const state of states) {
      await expect(page.getByRole('link', { name: state, exact: true })).toBeVisible();
    }
  });

  for (const state of states) {
    test(`${state} renders, passes axe and matches its baseline when asked`, async ({ page }) => {
      await page.goto(`/gallery?state=${state}`);
      // The heading for one state is the small uppercase label, so wait for the section itself.
      await expect(page.locator('section').first()).toBeVisible();

      const results = await new AxeBuilder({ page }).analyze();
      expect(results.violations).toEqual([]);

      await shot(page, `gallery-${state}.png`);
    });
  }
});
