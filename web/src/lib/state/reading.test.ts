import { describe, expect, it } from 'vitest';
import type { Entry, Revision } from '../api/client';
import {
  ageText,
  changeBetween,
  countText,
  deletedMeta,
  describeGroups,
  completionBadge,
  dateLabel,
  entryMeta,
  groupByMonth,
  otherEntries,
  questionAnswers,
  revisitMeta,
  sectionsFor,
  shelfEmpty,
  shelfOrder,
  SHELF_TABS,
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

  it('are month headings only in the date order: highest rated is one list, because the months would march backwards', () => {
    const entries = [
      entry({ id: 'a', rating_q: 20, finished_on: '2026-10-01' }),
      entry({ id: 'b', rating_q: 19, finished_on: '2024-01-01' }),
      entry({ id: 'c', rating_q: 18, finished_on: '2026-06-01' }),
    ];
    const sections = sectionsFor(entries, 'rating');
    expect(sections).toHaveLength(1);
    expect(sections[0].label).toBe('');
    expect(sections[0].items.map((e) => e.id)).toEqual(['a', 'b', 'c']);
    expect(sectionsFor(entries, 'newest').map((section) => section.label)).toEqual([
      'October 2026',
      'January 2024',
      'June 2026',
    ]);
    expect(sectionsFor([], 'rating')).toEqual([]);
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

describe('a row on one of the three shelves', () => {
  it('says a stopped row was stopped, and on what day', () => {
    expect(entryMeta(entry({ completion: 'abandoned' }), 2026)).toBe('Book · stopped 30 Oct');
    expect(entryMeta(entry({ completion: 'abandoned', finished_on: '2024-03-12' }), 2026)).toBe(
      'Book · stopped 12 Mar 2024',
    );
  });

  it('puts the creator where the date would be on a wanted row, because it helps you pick', () => {
    const wish = entry({
      completion: 'want_to',
      finished_on: null,
      work: {
        id: 'w1',
        medium: 'book',
        title: 'Gideon the Ninth',
        verified: true,
        creators: [{ role: 'author', name: 'Tamsyn Muir' }],
      },
    });
    expect(entryMeta(wish, 2026)).toBe('Book · Tamsyn Muir');
  });

  it('is the medium alone for a wanted row with no creator, rather than a stray dot', () => {
    expect(entryMeta(entry({ completion: 'want_to', finished_on: null }), 2026)).toBe('Book');
  });

  it('states the order on the two shelves that have nothing to rank', () => {
    // Finished ranks, so its order is a toggle she can flip.
    expect(shelfOrder('finished', 'newest')).toEqual({ label: 'Newest first', toggle: true });
    expect(shelfOrder('finished', 'rating')).toEqual({ label: 'Highest rated', toggle: true });
    // Bad: a rating order is never offered where the rows are unrated, because there is nothing to rank.
    expect(shelfOrder('abandoned', 'newest')).toEqual({ label: 'Newest first', toggle: false });
    expect(shelfOrder('abandoned', 'rating')).toEqual({ label: 'Newest first', toggle: false });
    expect(shelfOrder('want_to', 'newest')).toEqual({ label: 'Recently added', toggle: false });
    expect(shelfOrder('want_to', 'rating')).toEqual({ label: 'Recently added', toggle: false });
  });

  it('is one word per shelf, everywhere', () => {
    expect(SHELF_TABS.map((tab) => tab.label)).toEqual(['Finished', "Didn't finish", 'Wanted']);
    expect(SHELF_TABS.map((tab) => tab.shelf)).toEqual(['finished', 'abandoned', 'want_to']);
    // The same three words the badge and the card's switch use, so a tab and a badge cannot disagree.
    const labels = SHELF_TABS.map((tab) => tab.label as string);
    expect(labels).toContain(completionBadge(entry({ completion: 'want_to' })));
    expect(labels).toContain(completionBadge(entry({ completion: 'abandoned' })));
  });
});

describe('an empty shelf', () => {
  it('says what each shelf is for, and offers the one thing that fills it', () => {
    expect(shelfEmpty('abandoned', null)).toEqual({
      heading: 'Nothing left unfinished.',
      body: 'When you stop partway through something, say so and it lands here, with how far you got.',
      action: 'Log something',
    });
    expect(shelfEmpty('want_to', null)).toEqual({
      heading: 'Nothing on your wanted list yet.',
      body: 'Say what you mean to read, watch or play and it lands here, with no rating and nothing to write yet.',
      action: 'Add something',
    });
    // Finished keeps the library's own words, because it is the shelf that was always there.
    expect(shelfEmpty('finished', null).heading).toBe('Nothing here yet.');
  });

  it('names the shelf and the chip when a filter is on, and offers the way back', () => {
    const filtered = shelfEmpty('abandoned', { chosen: 'Podcast' });
    expect(filtered.heading).toBe('No podcast entries you stopped yet.');
    expect(filtered.action).toBe('Show everything');
    expect(shelfEmpty('want_to', { chosen: 'Book' }).heading).toBe(
      'No book entries on your wanted list yet.',
    );
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

describe('the shelves', () => {
  it('badges only the rows that are not an ordinary finished review', () => {
    expect(completionBadge(entry({ completion: 'finished' }))).toBe('');
    expect(completionBadge(entry({ completion: 'abandoned' }))).toBe("Didn't finish");
    expect(completionBadge(entry({ completion: 'want_to' }))).toBe('Wanted');
  });

  it('calls the date what it is: finished for a review, stopped for a work she put down', () => {
    expect(dateLabel(entry({ completion: 'finished' }))).toBe('Finished');
    expect(dateLabel(entry({ completion: 'want_to' }))).toBe('Finished');
    expect(dateLabel(entry({ completion: 'abandoned' }))).toBe('Stopped');
  });

  it('shows no date for a work she has not read, because there is not one', () => {
    expect(entryMeta(entry({ completion: 'want_to', finished_on: null }), 2026)).toBe('Book');
  });

  it('keeps the wanted shelf flat: a book she has not read has no month she finished it', () => {
    const rows = [
      entry({ id: 'a', completion: 'want_to', finished_on: null }),
      entry({ id: 'b', completion: 'want_to', finished_on: '2026-10-02' }),
    ];
    expect(sectionsFor(rows, 'newest', true)).toHaveLength(1);
    expect(sectionsFor(rows, 'newest', false).length).toBeGreaterThan(1);
  });

  it('names a shelf change and a how-far change in the version list', () => {
    expect(
      changeBetween(
        entry({ completion: 'finished' }),
        entry({ completion: 'abandoned', finished_on: '2026-10-30' }),
      ),
    ).toBe('Marked as not finished.');
    expect(changeBetween(entry({ progress: undefined }), entry({ progress: '100 pages' }))).toBe(
      'How far you got changed.',
    );
  });
});

describe('the revisit line', () => {
  it('describes a finished entry with its date and its stars', () => {
    const line = revisitMeta({ completion: 'finished', rating_q: 18, finished_on: '2026-11-02' });
    expect(line).toContain('finished');
    expect(line).toContain('4.5 stars');
    expect(line).not.toContain('0 stars');
  });

  it('never calls an unfinished row a zero-star finish', () => {
    const abandoned = revisitMeta({
      completion: 'abandoned',
      rating_q: 0,
      finished_on: '2026-11-02',
    });
    expect(abandoned).toContain('stopped');
    expect(abandoned).not.toContain('finished');
    expect(abandoned).not.toContain('stars');
    expect(revisitMeta({ completion: 'abandoned', rating_q: 0 })).toBe("didn't finish");
  });

  it('says wanted for a work she has not read', () => {
    expect(revisitMeta({ completion: 'want_to' })).toBe('wanted');
    expect(revisitMeta({ completion: 'want_to', rating_q: 0 })).not.toContain('stars');
  });
});
