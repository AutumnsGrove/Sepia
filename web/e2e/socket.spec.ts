import { expect, test } from '@playwright/test';

/**
 * The socket machine against a real socket and the real service. The Vitest suite proves the transition table
 * with a fake; this proves the fake is not lying about a browser WebSocket, the handshake, or the CSP.
 *
 * The harness lives at `/gallery?state=socket` because it is a dev tool, not a screen: it creates a session,
 * connects, and counts what arrives.
 */

test.describe('the socket machine, live', () => {
  test('opens, holds, and receives the turn it started', async ({ page }) => {
    await page.goto('/gallery?state=socket');
    await expect(page.getByTestId('socket-state')).toHaveText('open', { timeout: 15_000 });

    const eventsBefore = Number(await page.getByTestId('socket-events').textContent());
    await page.getByRole('button', { name: 'Send a message' }).click();

    // The dev world answers: her message is echoed as a persisted event, and the turn does something visible.
    await expect(page.getByTestId('socket-events')).not.toHaveText(String(eventsBefore), {
      timeout: 20_000,
    });
    await expect(page.getByTestId('socket-last')).not.toHaveText('none yet');
  });

  test('closes and opens again on demand, umprompted by anything else', async ({ page }) => {
    await page.goto('/gallery?state=socket');
    await expect(page.getByTestId('socket-state')).toHaveText('open', { timeout: 15_000 });
    const opens = Number(await page.getByTestId('socket-opens').textContent());

    await page.getByRole('button', { name: 'Reconnect' }).click();

    // A second successful open is the machine's close-then-connect path against a real socket, not the fake's.
    await expect(page.getByTestId('socket-opens')).toHaveText(String(opens + 1), {
      timeout: 15_000,
    });
    await expect(page.getByTestId('socket-state')).toHaveText('open');
  });
});
