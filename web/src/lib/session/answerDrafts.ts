/**
 * The in-progress answer to an interaction, kept in sessionStorage under its id.
 *
 * A selection lives in the component until Done, and the component is remounted by any navigation: a phone's
 * swipe-back, a route change, a reload. The interaction is still open on the server, so losing the taps means she
 * comes back to an unanswered card that looks like it forgot everything. The draft is per tab, never leaves the
 * device, and is cleared the moment an answer is sent.
 */

export interface AnswerDraft {
  /** The category tab she was on. */
  picked?: string;
  /** Describe: category id to the chip values she picked. */
  chosen?: Record<string, string[]>;
  /** Questions: question id to what she chose or typed. */
  answers?: Record<string, string>;
}

export function draftKey(id: string): string {
  return `sepia.ix.${id}`;
}

/** loadDraft reads a draft, treating unreadable storage as no draft. A preference is not worth an exception. */
export function loadDraft(storage: Storage | null, id: string): AnswerDraft {
  if (!storage || !id) return {};
  try {
    const raw = storage.getItem(draftKey(id));
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) return {};
    return parsed as AnswerDraft;
  } catch {
    return {};
  }
}

export function saveDraft(storage: Storage | null, id: string, draft: AnswerDraft): void {
  if (!storage || !id) return;
  try {
    storage.setItem(draftKey(id), JSON.stringify(draft));
  } catch {
    // A full or disabled storage must not stop her from answering.
  }
}

export function clearDraft(storage: Storage | null, id: string): void {
  if (!storage || !id) return;
  try {
    storage.removeItem(draftKey(id));
  } catch {
    // see saveDraft
  }
}

/** The tab's storage, or null where there is none (a test, or a render before the browser exists). */
export function browserStorage(): Storage | null {
  return typeof window === 'undefined' ? null : window.sessionStorage;
}
