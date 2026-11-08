import type { Draft } from '../contract/events';
import { saveReason } from './copy';

/**
 * What Save is waiting for. The server decides it (`can_confirm` and `missing`); this module only turns the answer
 * into a state the bar can draw, so the button and the sentence beside it can never disagree.
 */
export interface SaveState {
  allowed: boolean;
  /** Empty when Save can run; otherwise the words beside the button. */
  reason: string;
  missing: string[];
}

export function saveState(draft: Draft | null): SaveState {
  if (draft === null) return { allowed: false, reason: '', missing: [] };
  const missing = draft.missing ?? [];
  if (draft.can_confirm === true) return { allowed: true, reason: '', missing };
  return { allowed: false, reason: saveReason(missing), missing };
}
