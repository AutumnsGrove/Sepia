import type { Entry } from '../api/client';
import type { SessionState } from '../state/session';

/**
 * What the screen needs from the route, and what the route needs from the screen. Kept in a plain module so the
 * gallery can render the same screen with fixtures and no socket, and the route can wire the real thing.
 */
export interface ScreenHandlers {
  onsend: (text: string) => void;
  onanswer: (interactionID: string, value: unknown) => void;
  onedit: (set: Record<string, string | number | boolean>) => void;
  oninclude: (segmentID: string) => void;
  onaccept: (fieldID: string) => void;
  onacceptall: () => void;
  onconfirm: () => void;
  ondiscard: () => void;
  onstop: () => void;
  onretry: () => void;
  oncancel: (clientMsgID: string) => void;
  onback: () => void;
  oneditSaved?: () => void;
  onlogAnother?: () => void;
  onmore?: () => void;
  onversions?: () => void;
}

/** A message the outbox is holding: typed offline, or waiting behind the turn in flight. */
export interface PendingMessage {
  id: string;
  text: string;
  queued: boolean;
}

/** The topbar's title: the work once it is known, and "New entry" before that. */
export function sessionTitle(state: SessionState): string {
  const title = state.draft?.work.title;
  if (title && title !== 'Untitled') return title;
  return 'New entry';
}

/** The mode the card draws in. A saved session shows the read-only card; a revision draft says so. */
export function cardMode(
  state: SessionState,
  savedEntry: Entry | null,
): 'draft' | 'editing' | 'saved' {
  if (state.status === 'saved') return 'saved';
  if (savedEntry !== null) return 'editing';
  return 'draft';
}
