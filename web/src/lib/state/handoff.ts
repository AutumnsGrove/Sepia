/**
 * The sentence she types on Home, carried across the navigation into the new session's socket.
 *
 * It is kept in memory rather than in the URL or storage: her words in a URL are in the browser's history and in the
 * server's request log, and a review is the one thing that never leaves the app (docs/BUILD.md invariant 5). A reload
 * loses it, which puts her back on Home with the composer in front of her rather than half a review on a screen she
 * did not expect.
 */

let pending: string | null = null;

export function stashFirstMessage(text: string): void {
  const trimmed = text.trim();
  pending = trimmed === '' ? null : trimmed;
}

/** The stashed sentence, taken: the second call returns nothing, so a reconnect cannot send it twice. */
export function takeFirstMessage(): string {
  const text = pending ?? '';
  pending = null;
  return text;
}

/** Whether anything is waiting, which a test and the composer's own state ask. */
export function hasFirstMessage(): boolean {
  return pending !== null;
}
