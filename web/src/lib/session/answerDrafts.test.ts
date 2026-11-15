import { describe, expect, it } from 'vitest';
import { DRAFT_TTL_MS, draftKey, loadDraft, saveDraft, type AnswerDraft } from './answerDrafts';

/** A Map-backed Storage, so the draft rules are testable without a browser. */
function fakeStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (key: string) => data.get(key) ?? null,
    key: (index: number) => [...data.keys()][index] ?? null,
    removeItem: (key: string) => void data.delete(key),
    setItem: (key: string, value: string) => void data.set(key, value),
  };
}

describe('answer drafts', () => {
  it('keeps a draft per interaction and reads it back', () => {
    const store = fakeStorage();
    const draft: AnswerDraft = { picked: 'characters', chosen: { characters: ['memorable'] } };
    saveDraft(store, 'ix_1', draft);
    saveDraft(store, 'ix_2', { answers: { ending: 'stuck the landing' } });
    expect(loadDraft(store, 'ix_1')).toMatchObject(draft);
    expect(loadDraft(store, 'ix_2')).toMatchObject({ answers: { ending: 'stuck the landing' } });
    expect(draftKey('ix_1')).toBe('sepia.ix.ix_1');
  });

  it('keeps the draft after a send, so a refused answer can be retried', () => {
    // The send is fire-and-forget; clearing here lost her picks when the answer did not arrive.
    const store = fakeStorage();
    saveDraft(store, 'ix_1', { chosen: { characters: ['memorable'] } });
    expect(loadDraft(store, 'ix_1')).toMatchObject({ chosen: { characters: ['memorable'] } });
  });

  it('ignores a draft untouched for a day', () => {
    const store = fakeStorage();
    store.setItem(
      draftKey('ix_1'),
      JSON.stringify({
        chosen: { characters: ['memorable'] },
        savedAt: Date.now() - DRAFT_TTL_MS - 1,
      }),
    );
    expect(loadDraft(store, 'ix_1')).toEqual({});
  });

  it('keeps a rating along with the picks', () => {
    const store = fakeStorage();
    saveDraft(store, 'ix_1', { rating: 14 });
    expect(loadDraft(store, 'ix_1')).toMatchObject({ rating: 14 });
  });

  it('treats unreadable storage or JSON as no draft', () => {
    const store = fakeStorage();
    store.setItem(draftKey('ix_1'), 'not json');
    expect(loadDraft(store, 'ix_1')).toEqual({});
    expect(loadDraft(null, 'ix_1')).toEqual({});
    expect(loadDraft(store, '')).toEqual({});
  });

  it('survives a storage that refuses to write', () => {
    const store = fakeStorage();
    store.setItem = () => {
      throw new Error('quota');
    };
    expect(() => saveDraft(store, 'ix_1', {})).not.toThrow();
  });
});
