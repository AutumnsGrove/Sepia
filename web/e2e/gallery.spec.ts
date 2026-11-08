import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * The gallery is the unit of visual regression (S10.3): one baseline per component state, one axe scan per
 * state, on a phone viewport. A pixel diff is evidence, not proof, so look at the diff before updating a
 * baseline; the behavioural assertions live in the Go and Vitest suites.
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
  'ix-gap',
  'ix-lookup',
  'screen-split',
  'screen-candidates',
  'screen-describe',
  'screen-draft',
  'screen-saved',
  'screen-unverified',
  'screen-fidelity',
  'screen-editing',
  'screen-missing-rating',
  'screen-offline',
  'screen-caught-up',
  'screen-working',
  'screen-queued',
  'screen-error',
  'screen-preread-down',
  'screen-lookup-failed',
  'screen-hostile',
  'screen-stale',
];

test.describe('the gallery', () => {
  test('lists every state', async ({ page }) => {
    await page.goto('/gallery');
    await expect(page.getByRole('heading', { level: 1, name: 'Gallery' })).toBeVisible();
    for (const state of states) {
      await expect(page.getByRole('link', { name: state, exact: true })).toBeVisible();
    }
  });

  for (const state of states) {
    test(`${state} matches its baseline and passes axe`, async ({ page }) => {
      await page.goto(`/gallery?state=${state}`);
      // The heading for one state is the small uppercase label, so wait for the section itself.
      await expect(page.locator('section').first()).toBeVisible();

      const results = await new AxeBuilder({ page }).analyze();
      expect(results.violations).toEqual([]);

      await expect(page).toHaveScreenshot(`gallery-${state}.png`, { fullPage: true });
    });
  }
});
