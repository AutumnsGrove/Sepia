import { describe, expect, it } from 'vitest';
import { appendList, draftsOf, emptyList, replaceList, savedOf, toggle } from './lists';
import type { Session } from '../api/client';

interface Row {
  id: string;
  title: string;
}

const row = (id: string): Row => ({ id, title: `row ${id}` });
const key = (r: Row) => r.id;

function session(overrides: Partial<Session>): Session {
  return {
    id: 's1',
    kind: 'entry',
    state: 'active',
    title: null,
    created_at: '2026-11-08T12:00:00Z',
    updated_at: '2026-11-08T12:00:00Z',
    ...overrides,
  } as Session;
}

describe('list merging', () => {
  it('replaces the list on a refresh', () => {
    const state = replaceList({ items: [row('1'), row('2')], next_cursor: 'c2' });
    expect(state.items.map(key)).toEqual(['1', '2']);
    expect(state.cursor).toBe('c2');
    expect(state.hasMore).toBe(true);
  });

  it('appends a page and keeps the order', () => {
    const state = appendList(
      replaceList({ items: [row('1'), row('2')], next_cursor: 'c2' }),
      { items: [row('3'), row('4')], next_cursor: null },
      key,
    );
    expect(state.items.map(key)).toEqual(['1', '2', '3', '4']);
    expect(state.cursor).toBeNull();
    expect(state.hasMore).toBe(false);
  });

  it('never shows an item twice when a refresh overlaps a load more', () => {
    const first = replaceList({ items: [row('1'), row('2')], next_cursor: 'c2' });
    const overlapping = appendList(first, { items: [row('2'), row('3')], next_cursor: 'c3' }, key);
    expect(overlapping.items.map(key)).toEqual(['1', '2', '3']);
  });

  it('starts empty and says so', () => {
    expect(emptyList<Row>()).toEqual({ items: [], cursor: null, hasMore: false });
  });
});

describe('the session lists', () => {
  it('keeps active entry sessions as drafts and saved ones as the library', () => {
    const sessions = [
      session({ id: 'draft', kind: 'entry', state: 'active' }),
      session({ id: 'saved', kind: 'entry', state: 'saved' }),
      session({ id: 'chat', kind: 'general', state: 'active' }),
      session({ id: 'gone', kind: 'entry', state: 'discarded' }),
    ];
    expect(draftsOf(sessions).map((s) => s.id)).toEqual(['draft']);
    expect(savedOf(sessions).map((s) => s.id)).toEqual(['saved']);
  });
});

describe('chip selection', () => {
  it('toggles one value without touching the rest', () => {
    const one = toggle(new Set<string>(), 'dry');
    expect([...one]).toEqual(['dry']);
    const two = toggle(one, 'slow');
    expect([...two].sort()).toEqual(['dry', 'slow']);
    expect([...toggle(two, 'dry')]).toEqual(['slow']);
  });

  it('returns a new set, so a component cannot mutate the old one by accident', () => {
    const before = new Set(['a']);
    const after = toggle(before, 'b');
    expect(before.has('b')).toBe(false);
    expect(after.has('b')).toBe(true);
  });
});
