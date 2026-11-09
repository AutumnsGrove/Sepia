import { fileURLToPath } from 'node:url';
import { defineConfig, devices } from '@playwright/test';

// The repository root, so the server command never depends on where Playwright was started from.
const repoRoot = fileURLToPath(new URL('..', import.meta.url));

/**
 * End-to-end tests drive the real binary and the world, never a live provider.
 *
 * The server is `cmd/sepia-dev`: the real service against in-process fakes, on a port of its own so a
 * developer's running copy on 8893 is never disturbed. A phone viewport is the first target, because the
 * app is used on a phone and the mockups are frames of one.
 */
const PORT = 18893;
const BASE_URL = `http://localhost:${PORT}`;
// A second stack on its own port, with the model down: the sad route needs a real failure.
const FAULT_PORT = 18894;
const FAULT_URL = `http://localhost:${FAULT_PORT}`;
// The reading screens' stacks: seeded with an invented library, which the logging stack must not be. One per browser
// project, because the spec deletes and restores entries: two projects sharing a library would race each other's
// assertions and each other's screenshots.
const READING_PORT = 18895;
const READING_URL = `http://localhost:${READING_PORT}`;
const READING_WEBKIT_PORT = 18896;
const READING_WEBKIT_URL = `http://localhost:${READING_WEBKIT_PORT}`;

export { BASE_URL, FAULT_URL, READING_URL, READING_WEBKIT_URL };

export default defineConfig({
  testDir: 'e2e',
  // One baseline per screen, browser and platform. The same fonts rasterise differently on macOS and on the
  // Linux CI runner, and the potato is Linux, so CI's copy is the one that matters. A baseline is regenerated
  // on purpose by the `baselines` workflow, never by running --update-snapshots on whatever machine is handy.
  snapshotPathTemplate:
    '{testDir}/__screenshots__/{testFileName}/{arg}-{projectName}-{platform}{ext}',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],
  expect: {
    // A pixel diff is evidence, not proof: the Go tests carry the behavioural assertions. Keep this tight so
    // a real visual change is caught, and always look at the diff before updating a baseline.
    toHaveScreenshot: { maxDiffPixelRatio: 0.01, animations: 'disabled' },
  },
  use: {
    baseURL: BASE_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // A phone first, and reduced motion so a timestamp or an animation can never make a diff flaky.
    ...devices['iPhone 13'],
    viewport: { width: 390, height: 844 },
    reducedMotion: 'reduce',
    colorScheme: 'dark',
  },
  projects: [
    {
      name: 'chromium-phone',
      use: {
        ...devices['iPhone 13'],
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
      },
    },
    {
      name: 'webkit-phone',
      use: { ...devices['iPhone 13'], browserName: 'webkit' },
    },
  ],
  webServer: [
    {
      // A scratch data directory, removed first: the end-to-end suite must not start on last run's database,
      // and an earlier run's rows are what surfaced the unscoped trigger probe (docs/BUILD-LOG.md).
      command:
        'rm -rf /tmp/sepia-e2e-data && go build -o /tmp/sepia-e2e ./cmd/sepia-dev && exec /tmp/sepia-e2e',
      cwd: repoRoot,
      url: `${BASE_URL}/healthz`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        SEPIA_ADDR: `127.0.0.1:${PORT}`,
        SEPIA_ALLOWED_HOSTS: `localhost:${PORT},127.0.0.1:${PORT}`,
        SEPIA_DATA_DIR: '/tmp/sepia-e2e-data',
        SEPIA_DEV: '1',
        // The whole logging flow, one card at a time: medium, candidate, Describe, questions, card.
        SEPIA_DEV_SCENARIO: 'split',
      },
    },
    {
      // The reading screens' stack: the same service, seeded with invented reviews through the engine, so a list, a
      // version history and a deleted entry are real rows rather than fixtures.
      command:
        'rm -rf /tmp/sepia-e2e-reading-data && go build -o /tmp/sepia-e2e-reading ./cmd/sepia-dev && exec /tmp/sepia-e2e-reading',
      cwd: repoRoot,
      url: `${READING_URL}/healthz`,
      // Never reused: the stack is seeded on start and two tests change it (a delete, a draft opened from a
      // version), so a second run against the first run's data is a run against a different library.
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        SEPIA_ADDR: `127.0.0.1:${READING_PORT}`,
        SEPIA_ALLOWED_HOSTS: `localhost:${READING_PORT},127.0.0.1:${READING_PORT}`,
        SEPIA_DATA_DIR: '/tmp/sepia-e2e-reading-data',
        SEPIA_DEV: '1',
        SEPIA_DEV_SEED: 'library',
      },
    },
    {
      // The same seeded stack, on its own port, for the second browser project.
      command:
        'rm -rf /tmp/sepia-e2e-reading-webkit-data && go build -o /tmp/sepia-e2e-reading-webkit ./cmd/sepia-dev && exec /tmp/sepia-e2e-reading-webkit',
      cwd: repoRoot,
      url: `${READING_WEBKIT_URL}/healthz`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        SEPIA_ADDR: `127.0.0.1:${READING_WEBKIT_PORT}`,
        SEPIA_ALLOWED_HOSTS: `localhost:${READING_WEBKIT_PORT},127.0.0.1:${READING_WEBKIT_PORT}`,
        SEPIA_DATA_DIR: '/tmp/sepia-e2e-reading-webkit-data',
        SEPIA_DEV: '1',
        SEPIA_DEV_SEED: 'library',
      },
    },
    {
      // The same stack with the model down, so the sad route is a real error and a real Try again rather than a
      // fixture: a screen that only ever sees a working world is not tested.
      command:
        'rm -rf /tmp/sepia-e2e-fault-data && go build -o /tmp/sepia-e2e-fault ./cmd/sepia-dev && exec /tmp/sepia-e2e-fault',
      cwd: repoRoot,
      url: `${FAULT_URL}/healthz`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        SEPIA_ADDR: `127.0.0.1:${FAULT_PORT}`,
        SEPIA_ALLOWED_HOSTS: `localhost:${FAULT_PORT},127.0.0.1:${FAULT_PORT}`,
        SEPIA_DATA_DIR: '/tmp/sepia-e2e-fault-data',
        SEPIA_DEV: '1',
        SEPIA_DEV_SCENARIO: 'split',
        SEPIA_DEV_FAULT: 'chat',
      },
    },
  ],
});
