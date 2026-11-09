import { describe, expect, it } from 'vitest';
import type { Entry, Revision } from '../api/client';
import {
  ageText,
  changeBetween,
  countText,
  deletedMeta,
  describeGroups,
  entryMeta,
  groupByMonth,
  otherEntries,
  questionAnswers,
  versionRows,
  waitingText,
  warningsOf,
} from './reading';

/**
 * The reading screens' derived state. The routes below are the ones in the slice plan: the happy shape, the hostile or
 * missing input, and the leaving-nothing-out cases a list is judged on (an entry with no date, a draft with no reason,
 * a version that changed nothing visible).
 */

function entry(overrides: Partial<Entry> = {}): Entry {
  return {
    id: 'e1',
    work: { id: 'w1', medium: 'book', title: 'The Lantern Keeper', verified: true },
    rating_q: 18,
    rating_scale: 'quarter_stars_20',
    favourite: false,
    body: 'the middle dragged a bit',
    body_origin: 'typed',
    status: 'saved',
    format: 'paperback',
    finished_on: '2026-10-30',
    created_at: '2026-10-30T18:00:00Z',
    updated_at: '2026-10-30T18:00:00Z',
    ...overrides,
  } as Entry;
}

describe('the month groups', () => {
  it('groups a page by the month it was finished, keeping the server order', () => {
    const groups = groupByMonth([
      entry({ id: 'a', finished_on: '2026-10-30' }),
      entry({ id: 'b', finished_on: '2026-09-28' }),
      entry({ id: 'c', finished_on: '2026-10-02' }),
    ]);
    expect(groups.map((group) => group.label)).toEqual(['October 2026', 'September 2026']);
    expect(groups[0].items.map((e) => e.id)).toEqual(['a', 'c']);
  });

  it('does not drop an entry with no finish date, and puts it last', () => {
    const groups = groupByMonth([
      entry({ id: 'a', finished_on: '2026-10-30' }),
      entry({ id: 'b', finished_on: null }),
    ]);
    expect(groups.map((group) => group.key)).toEqual(['2026-10', '']);
    expect(groups[1].label).toBe('No date');
    expect(groups[1].items[0].id).toBe('b');
  });

  it('of an empty library is no groups, rather than a group with nothing in it', () => {
    expect(groupByMonth([])).toEqual([]);
  });
});

describe('a row meta line', () => {
  it('names the medium and the day, with the year when it is not this one', () => {
    expect(entryMeta(entry(), 2026)).toBe('Book · 30 Oct');
    expect(entryMeta(entry({ finished_on: '2024-03-12' }), 2026)).toBe('Book · 12 Mar 2024');
  });

  it('is the medium alone with no date, rather than a stray dot', () => {
    expect(entryMeta(entry({ finished_on: null }), 2026)).toBe('Book');
  });

  it('keeps a date it cannot parse rather than showing nothing', () => {
    expect(entryMeta(entry({ finished_on: 'yesterday' }), 2026)).toContain('yesterday');
  });
});

describe('the deleted row', () => {
  it('says when it happened', () => {
    expect(deletedMeta(entry({ deleted_at: '2026-10-30T21:00:00Z' }), 2026)).toBe(
      'Deleted Fri 30 Oct',
    );
  });

  it('says so plainly when the instant is missing, rather than inventing a date', () => {
    expect(deletedMeta(entry(), 2026)).toBe('Deleted');
  });
});

describe('the drafts list', () => {
  it('says what each reason means in words', () => {
    expect(waitingText('needs_rating')).toBe('Needs a rating');
    expect(waitingText('pick_work')).toBe('Waiting for you to pick which one');
    expect(waitingText('needs_date')).toBe('Needs a date');
  });

  it('has an answer for a reason it does not know, so a row is never blank', () => {
    expect(waitingText('something_new')).toBe('Waiting for you');
    expect(waitingText(null)).toBe('Waiting for you');
  });

  it('coarsens an age as it gets older', () => {
    const now = new Date('2026-10-31T12:00:00Z');
    expect(ageText('2026-10-31T11:59:30Z', now)).toBe('Just now');
    expect(ageText('2026-10-31T10:00:00Z', now)).toBe('2 h ago');
    expect(ageText('2026-10-30T09:00:00Z', now)).toBe('Yesterday');
    expect(ageText('2026-10-28T12:00:00Z', now)).toBe('3 days ago');
    expect(ageText('2026-09-01T12:00:00Z', now)).toBe('1 Sept');
    expect(ageText(null, now)).toBe('');
  });
});

describe('the counts', () => {
  it('is the number, or 99+ when the page was full', () => {
    expect(countText(2)).toBe('2');
    expect(countText(0)).toBe('0');
    expect(countText(99)).toBe('99');
    expect(countText(100)).toBe('99+');
  });
});

describe('the entry page', () => {
  const answers = [
    {
      question_id: 'characters',
      category: 'characters',
      prompt: 'Characters',
      answer_type: 'tags' as const,
      values: ['memorable', 'multilayered'],
    },
    {
      question_id: 'content_warnings',
      category: 'content_warnings',
      prompt: 'Content warnings',
      answer_type: 'tags' as const,
      values: ['violence', 'war'],
    },
    {
      question_id: 'ft_ending',
      prompt: 'How did the ending land?',
      answer_type: 'choice' as const,
      values: ['it landed'],
    },
    {
      question_id: 'ft_length',
      prompt: 'Was it the right length?',
      answer_type: 'choice' as const,
      values: [],
      skipped: true,
    },
  ];

  it('groups the chips by category and keeps the warnings apart', () => {
    expect(describeGroups(answers)).toEqual([
      { category: 'characters', label: 'Characters', values: ['memorable', 'multilayered'] },
    ]);
    expect(warningsOf(answers)).toEqual(['violence', 'war']);
  });

  it('shows the questions she answered and none of the ones she skipped', () => {
    expect(questionAnswers(answers)).toEqual([
      { prompt: 'How did the ending land?', values: ['it landed'] },
    ]);
  });

  it('leaves out the entry being read from the work timeline', () => {
    const timeline = otherEntries([entry({ id: 'a' }), entry({ id: 'b' })], 'a');
    expect(timeline.map((e) => e.id)).toEqual(['b']);
  });
});

describe('the versions screen', () => {
  function snapshot(overrides: Partial<Entry> = {}): Entry {
    return entry({ id: 'e1', ...overrides });
  }

  it('lists the current version first and every one it replaced, numbered down to one', () => {
    const revisions = [
      {
        id: 'r2',
        entry_id: 'e1',
        revision: 2,
        snapshot: snapshot({ rating_q: 17, body: 'first thoughts' }),
        created_at: '2026-10-31T18:00:00Z',
      },
      {
        id: 'r1',
        entry_id: 'e1',
        revision: 1,
        snapshot: snapshot({ rating_q: 15, body: 'first thoughts' }),
        created_at: '2026-10-30T21:14:00Z',
      },
    ] satisfies Revision[];
    const rows = versionRows(snapshot({ rating_q: 19 }), revisions);
    expect(rows.map((row) => row.number)).toEqual([3, 2, 1]);
    expect(rows[0].current).toBe(true);
    // The current version changed the rating and the words, which is both of what it did.
    expect(rows[0].change).toBe('Rating 4.25 to 4.75. Words changed.');
    expect(rows[1].change).toBe('Rating 3.75 to 4.25.');
    expect(rows[2].change).toBe('First saved.');
  });

  it('of a first save is one row that says so', () => {
    const rows = versionRows(snapshot(), []);
    expect(rows).toHaveLength(1);
    expect(rows[0].change).toBe('First saved.');
  });

  it('says what changed in words that are true of the card', () => {
    expect(changeBetween(snapshot({ rating_q: 18 }), snapshot({ rating_q: 19 }))).toBe(
      'Rating 4.5 to 4.75.',
    );
    expect(changeBetween(snapshot({ body: 'a b' }), snapshot({ body: 'a b and c' }))).toBe(
      'Words extended.',
    );
    expect(changeBetween(snapshot({ body: 'a b and c' }), snapshot({ body: 'a b' }))).toBe(
      'Words shortened.',
    );
    expect(changeBetween(snapshot({ format: '' }), snapshot({ format: 'print' }))).toBe(
      'Format set to Print.',
    );
    expect(changeBetween(snapshot(), snapshot())).toBe('No visible change.');
  });
});
