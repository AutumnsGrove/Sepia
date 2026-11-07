import { expect, test } from '@playwright/test';

/**
 * The initial budget (S10 exit): what a phone downloads to show the app shell, compressed, before it does
 * anything. The numbers are asserted rather than admired, so a dependency that doubles the bundle fails a
 * test instead of being noticed months later.
 */

// Measured when S10 landed: about 37 kB of JS and under 1 kB of CSS gzipped, before the screens exist.
// The budget is roughly double that, so S11 can add the card without failing, and a dependency that doubles
// the bundle fails here rather than on a phone.
const BUDGET = {
  js: 80_000,
  css: 12_000,
};

test('the initial build stays inside its budget', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('heading', { level: 1, name: 'Sepia' }).waitFor();

  const sizes = await page.evaluate(() => {
    const entries = performance.getEntriesByType('resource') as PerformanceResourceTiming[];
    let js = 0;
    let css = 0;
    for (const entry of entries) {
      // transferSize is what crossed the network; the server sends the precompressed copy.
      const bytes = entry.transferSize || entry.encodedBodySize || 0;
      if (entry.name.endsWith('.js')) js += bytes;
      if (entry.name.endsWith('.css')) css += bytes;
    }
    return { js, css };
  });

  expect(sizes.js, `initial JS is ${sizes.js} bytes`).toBeLessThanOrEqual(BUDGET.js);
  expect(sizes.css, `initial CSS is ${sizes.css} bytes`).toBeLessThanOrEqual(BUDGET.css);
});
