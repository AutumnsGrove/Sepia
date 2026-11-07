import type { Session } from '../api/client';

/**
 * Lists that come from REST rather than from the socket: the Drafts page and recent entries. They are derived
 * state: a component reads `items` and asks for more; it never pushes into the array. Merging is by key so a
 * refresh that overlaps a "load more" cannot show the same entry twice.
 */

export interface Page<T> {
  items: T[];
  next_cursor: string | null;
}

export interface ListState<T> {
  items: T[];
  /** The cursor for the next page, or null at the end of the list. */
  cursor: string | null;
  hasMore: boolean;
}

export function emptyList<T>(): ListState<T> {
  return { items: [], cursor: null, hasMore: false };
}

/** A refresh replaces the page: the server's newest order is the truth. */
export function replaceList<T>(page: Page<T>): ListState<T> {
  return { items: page.items, cursor: page.next_cursor, hasMore: page.next_cursor !== null };
}

/** A "load more" appends, dropping anything already shown. */
export function appendList<T>(
  state: ListState<T>,
  page: Page<T>,
  key: (item: T) => string,
): ListState<T> {
  const seen = new Set(state.items.map(key));
  const merged = state.items.slice();
  for (const item of page.items) {
    const id = key(item);
    if (seen.has(id)) continue;
    seen.add(id);
    merged.push(item);
  }
  return { items: merged, cursor: page.next_cursor, hasMore: page.next_cursor !== null };
}

/** The Drafts list is active entry sessions: a general chat is not a draft of a review. */
export function draftsOf(sessions: Session[]): Session[] {
  return sessions.filter((session) => session.kind === 'entry' && session.state === 'active');
}

/** The library's "recent" strip: saved entries, newest first, as the server already ordered them. */
export function savedOf(sessions: Session[]): Session[] {
  return sessions.filter((session) => session.state === 'saved');
}

/** A count that a tab can show without walking the list twice. */
export function toggle(chips: Set<string>, value: string): Set<string> {
  const next = new Set(chips);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}
