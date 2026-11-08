import { describe, expect, it } from 'vitest';
import type { Entry } from '../api/client';
import type { Draft } from '../contract/events';
import { whatWillChange } from './diff';

const work = {
  id: 'w1',
  title: 'The Lantern Keeper',
  medium: 'book' as const,
  verified: true,
};

const saved: Entry = {
  id: 'e1',
  work,
  rating_q: 18,
  rating_scale: 'quarter_stars_20',
  favourite: false,
  verdict: 'the ending landed',
  body: 'the ending landed but it was too long',
  body_origin: 'extracted',
  status: 'saved',
  format: 'paperback',
  finished_on: '2026-10-30',
  tags: [{ name: 'lighthouses', source: 'genre' }],
  created_at: '2026-10-30T12:00:00Z',
  updated_at: '2026-10-30T12:00:00Z',
};

function draft(patch: Partial<Draft> = {}): Draft {
  return {
    version: 3,
    work,
    rating: { quarters: 18, scale: 'quarter_stars_20', favourite: false },
    verdict: 'the ending landed',
    body: 'the ending landed but it was too long',
    body_origin: 'extracted',
    format: 'paperback',
    finished_on: '2026-10-30',
    blocks: {
      work: 'verified',
      rating: 'set',
      body: 'extracted',
      facts: 'parsed',
      answers: 'empty',
    },
    tags: [{ name: 'lighthouses', source: 'genre' }],
    ...patch,
  };
}

describe('what will change', () => {
  it('lists nothing when nothing changed', () => {
    expect(whatWillChange(saved, draft())).toEqual([]);
  });

  it('lists the rating with the old value struck through', () => {
    const changes = whatWillChange(
      saved,
      draft({ rating: { quarters: 19, scale: 'quarter_stars_20', favourite: false } }),
    );
    expect(changes).toEqual([{ field: 'rating', label: 'Rating', was: '4.5', now: '4.75' }]);
  });

  it('says a body is edited rather than printing two paragraphs', () => {
    const changes = whatWillChange(saved, draft({ body: 'a different thought' }));
    expect(changes).toEqual([{ field: 'body', label: 'Your words', now: 'edited' }]);
  });

  it('keeps the card order when several fields moved', () => {
    const changes = whatWillChange(
      saved,
      draft({
        rating: { quarters: 20, scale: 'quarter_stars_20', favourite: true },
        verdict: 'stayed with me',
        body: 'a different thought',
        format: 'hardback',
        finished_on: '2026-11-01',
        tags: [{ name: 'memory', source: 'mine' }],
      }),
    );
    expect(changes.map((change) => change.field)).toEqual([
      'rating',
      'verdict',
      'body',
      'format',
      'finished_on',
      'favourite',
      'tags',
    ]);
    expect(changes[0].now).toBe('5');
    expect(changes[3].now).toBe('hardback');
    expect(changes[5].now).toBe('Yes');
    expect(changes[6].now).toBe('memory');
  });

  it('treats a cleared optional field as a change, not as nothing', () => {
    const changes = whatWillChange(saved, draft({ format: '', finished_on: undefined }));
    expect(changes.map((change) => change.now)).toEqual(['Not set', 'Not set']);
  });
});
