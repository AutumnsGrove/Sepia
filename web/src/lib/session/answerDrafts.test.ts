import { describe, expect, it } from 'vitest';
import { clearDraft, draftKey, loadDraft, saveDraft, type AnswerDraft } from './answerDrafts';

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
    expect(loadDraft(store, 'ix_1')).toEqual(draft);
    expect(loadDraft(store, 'ix_2')).toEqual({ answers: { ending: 'stuck the landing' } });
    expect(draftKey('ix_1')).toBe('sepia.ix.ix_1');
  });

  it('clears a draft when the answer has been sent', () => {
    const store = fakeStorage();
    saveDraft(store, 'ix_1', { chosen: { characters: ['memorable'] } });
    clearDraft(store, 'ix_1');
    expect(loadDraft(store, 'ix_1')).toEqual({});
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
    expect(() => clearDraft(store, 'ix_1')).not.toThrow();
  });
});
