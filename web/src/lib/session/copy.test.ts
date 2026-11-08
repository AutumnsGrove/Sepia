import { describe, expect, it } from 'vitest';
import { errorText, lookupSentence, saveReason, uiCopy } from './copy';

describe('the copy this screen adds', () => {
  it('says what is missing in the order the card lists it', () => {
    expect(saveReason(['rating'])).toBe('Add a rating to save');
    expect(saveReason(['body'])).toBe('Add a few words of your own to save');
    expect(saveReason(['rating', 'body'])).toBe('Add a rating and a few words of your own to save');
    expect(saveReason([])).toBe('');
  });

  it('uses the contract words for a known code and the server words for an unknown one', () => {
    expect(errorText('stale_version', 'whatever the server said')).toBe(
      'The card changed while you were editing. Here it is again.',
    );
    expect(errorText('model_error', 'The model failed.')).toBe('The model failed.');
  });

  it('names the service that did not answer, and never leaves a blank', () => {
    expect(lookupSentence('TMDB')).toBe(
      'TMDB did not answer in time. Your message is saved as a draft.',
    );
    expect(lookupSentence(undefined)).toContain('The metadata service');
  });

  it('counts fields and segments so a single one is not plural', () => {
    expect(uiCopy.acceptAll(1)).toBe('1 field from the web');
    expect(uiCopy.acceptAll(2)).toBe('2 fields from the web');
    expect(uiCopy.leftOut(1)).toBe('Left out (1)');
    expect(uiCopy.earlierVersions(1)).toBe('1 earlier version');
    expect(uiCopy.earlierVersions(3)).toBe('3 earlier versions');
  });

  it('uses no em dash, which is the one typography rule for user copy', () => {
    for (const value of Object.values(uiCopy)) {
      if (typeof value === 'string') expect(value).not.toContain('—');
    }
  });
});
