import type { Entry } from '../api/client';
import type { Draft } from '../contract/events';
import { dateLabel } from '../state/reading';
import { dateText, ratingText } from './format';

/**
 * "What will change" when a saved entry is reopened. Only fields that actually differ are listed, in the card's
 * order, and the words are the card's own: a diff that says "edited" for a body is honest, because showing two
 * long paragraphs side by side on a phone is not a diff anyone reads.
 */
export interface Change {
  field:
    | 'rating'
    | 'body'
    | 'verdict'
    | 'format'
    | 'finished_on'
    | 'favourite'
    | 'tags'
    | 'completion'
    | 'progress';
  label: string;
  /** The old value, when it is short enough to show. */
  was?: string;
  now: string;
}

export function whatWillChange(saved: Entry, draft: Draft): Change[] {
  const changes: Change[] = [];

  const wasRating = ratingText(saved.rating_q);
  const nowRating = ratingText(draft.rating.quarters);
  if (wasRating !== nowRating) {
    changes.push({ field: 'rating', label: 'Rating', was: wasRating, now: nowRating });
  }

  if ((saved.verdict ?? '') !== (draft.verdict ?? '')) {
    changes.push({
      field: 'verdict',
      label: 'Verdict',
      was: saved.verdict || 'None',
      now: draft.verdict || 'None',
    });
  }

  if (saved.body !== draft.body) {
    changes.push({ field: 'body', label: 'Your words', now: 'edited' });
  }

  const wasFormat = saved.format ?? 'Not set';
  const nowFormat = draft.format || 'Not set';
  if (wasFormat !== nowFormat) {
    changes.push({ field: 'format', label: 'Format', was: wasFormat, now: nowFormat });
  }

  const wasDate = dateText(saved.finished_on);
  const nowDate = dateText(draft.finished_on);
  if (wasDate !== nowDate) {
    changes.push({ field: 'finished_on', label: dateLabel(draft), was: wasDate, now: nowDate });
  }

  const wasShelf = shelfText(saved.completion);
  const nowShelf = shelfText(draft.completion);
  if (wasShelf !== nowShelf) {
    changes.push({ field: 'completion', label: 'Shelf', was: wasShelf, now: nowShelf });
  }

  const wasProgress = saved.progress ?? 'None';
  const nowProgress = draft.progress || 'None';
  if (wasProgress !== nowProgress) {
    changes.push({ field: 'progress', label: 'How far', was: wasProgress, now: nowProgress });
  }

  if (saved.favourite !== draft.rating.favourite) {
    changes.push({
      field: 'favourite',
      label: 'Favourite',
      was: saved.favourite ? 'Yes' : 'No',
      now: draft.rating.favourite ? 'Yes' : 'No',
    });
  }

  const wasTags = tagNames(saved.tags);
  const nowTags = tagNames(draft.tags);
  if (wasTags !== nowTags) {
    changes.push({
      field: 'tags',
      label: 'Tags',
      was: wasTags || 'None',
      now: nowTags || 'None',
    });
  }

  return changes;
}

function tagNames(tags: { name: string }[] | undefined): string {
  return (tags ?? []).map((tag) => tag.name).join(', ');
}

/** The shelf, in the plain words the card uses. A normal revision changes neither side, so it is never listed. */
function shelfText(completion: Draft['completion'] | Entry['completion'] | undefined): string {
  switch (completion) {
    case 'want_to':
      return 'Wanted';
    case 'abandoned':
      return "Didn't finish";
    default:
      return 'Finished';
  }
}
