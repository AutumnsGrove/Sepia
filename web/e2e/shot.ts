import { expect, type Locator, type Page } from '@playwright/test';

/**
 * A pixel baseline, compared only when `SCREENSHOTS=1` (`make screenshots`).
 *
 * The variable is deliberately not `SEPIA_`-prefixed: the dev server refuses an unknown `SEPIA_` key, and this one
 * rides along in the environment of every web server Playwright starts.
 *
 * CI runs the end-to-end suite for behaviour and axe and compares no pixels. A PNG is only true on the machine that
 * drew it, so a baseline made on the runner was one nobody ever developed against, and the ceremony that kept it
 * honest, a second workflow and a downloaded artifact, cost more than it caught. The PNGs in `__screenshots__/` belong
 * to whoever runs this, one set per platform, and are compared when asked for.
 */
export async function shot(
  page: Page,
  name: string,
  options: { fullPage?: boolean; mask?: Locator[] } = {},
): Promise<void> {
  if (process.env.SCREENSHOTS !== '1') return;
  await expect(page).toHaveScreenshot(name, { fullPage: true, ...options });
}
