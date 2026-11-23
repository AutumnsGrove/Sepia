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

/** The two question kinds that carry her answers; a card saved before they are resolved would go without them. */
export const ANSWER_KINDS = ['describe', 'question_form'] as const;

/** Whether a question that carries her answer is still open on the card (issue #46). */
export function waitingOnAnswer(kinds: readonly string[]): boolean {
  return kinds.some((kind) => (ANSWER_KINDS as readonly string[]).includes(kind));
}

export function saveState(draft: Draft | null, answerPending = ''): SaveState {
  if (draft === null) return { allowed: false, reason: '', missing: [] };
  const missing = draft.missing ?? [];
  if (answerPending !== '') return { allowed: false, reason: answerPending, missing };
  if (draft.can_confirm === true) return { allowed: true, reason: '', missing };
  return { allowed: false, reason: saveReason(missing), missing };
}
