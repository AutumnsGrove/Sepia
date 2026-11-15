/**
 * The in-progress answer to an interaction, kept in sessionStorage under its id.
 *
 * A selection lives in the component until Done, and the component is remounted by any navigation: a phone's
 * swipe-back, a route change, a reload. The interaction is still open on the server, so losing the taps means she
 * comes back to an unanswered card that looks like it forgot everything.
 *
 * The draft is per tab and never leaves the device. It is deliberately not cleared when an answer is sent: the send
 * is fire-and-forget, so a refused or dropped answer would take her taps with it. Once the interaction resolves its
 * id is never offered again, and an untouched draft older than a day is ignored.
 */

export interface AnswerDraft {
  /** The category tab she was on. */
  picked?: string;
  /** Describe: category id to the chip values she picked. */
  chosen?: Record<string, string[]>;
  /** Questions: question id to what she chose or typed. */
  answers?: Record<string, string>;
  /** A fill-gap rating, in quarter stars, until she taps Set it. */
  rating?: number;
  /** When the draft was last touched, so a resolved interaction's leftovers can be ignored. */
  savedAt?: number;
}

/** After this long an untouched draft is from an interaction that has resolved; it is not shown again. */
export const DRAFT_TTL_MS = 24 * 60 * 60 * 1000;

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
    const draft = parsed as AnswerDraft;
    if (typeof draft.savedAt === 'number' && Date.now() - draft.savedAt > DRAFT_TTL_MS) return {};
    return draft;
  } catch {
    return {};
  }
}

export function saveDraft(storage: Storage | null, id: string, draft: AnswerDraft): void {
  if (!storage || !id) return;
  try {
    storage.setItem(draftKey(id), JSON.stringify({ ...draft, savedAt: Date.now() }));
  } catch {
    // A full or disabled storage must not stop her from answering.
  }
}

/** The tab's storage, or null where there is none (a test, or a render before the browser exists). */
export function browserStorage(): Storage | null {
  return typeof window === 'undefined' ? null : window.sessionStorage;
}
