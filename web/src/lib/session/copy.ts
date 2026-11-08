/**
 * The sentences the logging screen adds. Anything that is a stable error or notice code comes from the contract
 * (`copyFor`), never from here: this file is only for the words a code cannot carry, such as the queued line or
 * the reason a Save button is off. The table in docs/slices/S11-logging-ui.md is the review copy.
 */
import { copyFor, errorCodes } from '../contract/codes';

export const uiCopy = {
  offline: 'Offline. Reconnecting.',
  offlinePending: 'Will send when you are back online',
  backOnline: 'Back online. You did not miss anything.',
  queued: 'Queued, sends when this finishes',
  queuedHint: 'One message is already waiting.',
  waitForReply: 'Wait for the next reply',
  working: 'Working',
  editingSaved: 'You are editing a saved entry. Saving keeps the previous version.',
  attachDisabled: 'Screenshots arrive later',
  savedNote: 'Saved to your library',
  whyTitle: 'How this was read',
  whyAllowance: 'An allowance is not money spent.',
  unverified: 'From the web, unverified',
  imageRead: 'Read from the image',
  needed: 'Needed',
  acceptImage: 'Rating and date were read from the image',
  nothingYet: 'Nothing yet.',
  discard: 'Discard',
  save: 'Save entry',
  saveChanges: 'Save changes',
  cancel: 'Cancel',
  edit: 'Edit',
  logAnother: 'Log another',
  include: 'Include',
  earlierVersions: (n: number) => (n === 1 ? '1 earlier version' : `${n} earlier versions`),
  acceptAll: (n: number) => (n === 1 ? '1 field from the web' : `${n} fields from the web`),
  leftOut: (n: number) => `Left out (${n})`,
} as const;

/** The words over the confirm bar when Save cannot run. The server's `missing` list is the truth. */
export function saveReason(missing: string[]): string {
  const needsRating = missing.includes('rating');
  const needsBody = missing.includes('body');
  if (needsRating && needsBody) return 'Add a rating and a few words of your own to save';
  if (needsRating) return 'Add a rating to save';
  if (needsBody) return 'Add a few words of your own to save';
  return missing.length > 0 ? 'Something is still missing to save' : '';
}

/** A lookup failure is composed, because the service that failed is what she needs to hear. */
export function lookupSentence(service: string | undefined): string {
  return `${service || 'The metadata service'} did not answer in time. Your message is saved as a draft.`;
}

/**
 * An error's sentence: the contract's words for a code it knows, and the server's own message for one it does not
 * (a newer service may send a code this build has never heard of, and "Something went wrong" would hide it).
 */
export function errorText(code: string, message: string): string {
  const known = (errorCodes as Record<string, { copy: string }>)[code];
  return known ? known.copy : message;
}

/** The plain sentence for an error code, from the contract, with the composer's own refusal on top of it. */
export function problemText(
  problem: 'empty' | 'too_long' | 'queue_full' | 'session_closed',
): string {
  switch (problem) {
    case 'empty':
      return '';
    case 'too_long':
      return copyFor('message_too_long');
    case 'queue_full':
      return copyFor('queue_full');
    case 'session_closed':
      return copyFor('session_closed');
  }
}
