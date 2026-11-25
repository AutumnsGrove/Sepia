import { describe, expect, it } from 'vitest';
import {
  FORMATS,
  dateText,
  formatText,
  mediumText,
  providerText,
  ratingText,
  usdText,
  verdictText,
} from './format';

describe('the card format', () => {
  it('shows a quarter-star rating without a trailing zero', () => {
    expect(ratingText(18)).toBe('4.5');
    expect(ratingText(20)).toBe('5');
    expect(ratingText(1)).toBe('0.25');
    expect(ratingText(0)).toBe('Not rated');
  });

  it('shows money with four decimals under a cent and two above', () => {
    expect(usdText(0.0021)).toBe('$0.0021');
    expect(usdText(0.01)).toBe('$0.01');
    expect(usdText(0)).toBe('$0.00');
    expect(usdText(1.5)).toBe('$1.50');
  });

  it('reads a date in the mockups shape, at noon so no timezone can move the day', () => {
    expect(dateText('2026-10-30')).toBe('Fri 30 Oct');
    expect(dateText(null)).toBe('Not set');
    expect(dateText('not a date')).toBe('not a date');
  });

  it('names the medium, and says nothing for one it does not know', () => {
    expect(mediumText('book')).toBe('Book');
    expect(mediumText('tv')).toBe('TV');
    expect(mediumText('something-new')).toBe('');
  });

  it('names the format in plain words, from the same list the picker offers', () => {
    expect(formatText('audiobook')).toBe('Audiobook');
    expect(formatText('cinema')).toBe('Cinema');
    expect(formatText('')).toBe('');
    // The picker and the label read the one list, so a format cannot be offered but unnameable.
    for (const [value, label] of FORMATS) expect(formatText(value)).toBe(label);
  });

  it('names the provider, and never shows its id', () => {
    expect(providerText('openlibrary')).toBe('Open Library');
    expect(providerText('tmdb')).toBe('TMDB');
    expect(providerText('polaris')).toBe('Polaris');
    expect(providerText('manual')).toBe('Your own details');
    expect(providerText('a-new-provider')).toBe('');
  });

  it('drops a verdict the body already contains, so her words are printed once', () => {
    expect(verdictText('the ending landed', 'the ending landed but it was too long')).toBe('');
    expect(verdictText('the ending landed', 'the ending was too long')).toBe('the ending landed');
    expect(verdictText('', 'anything')).toBe('');
    expect(verdictText(undefined, undefined)).toBe('');
  });

  it('matches a verdict as words, so a short one is not hidden inside a longer word', () => {
    // "fine" is inside "defined" and "ok" inside "broken" as substrings, but neither is the same word.
    expect(verdictText('fine', 'the characters were well defined')).toBe('fine');
    expect(verdictText('ok', 'the middle was broken')).toBe('ok');
    // The same words still count through case and punctuation.
    expect(verdictText('The ending landed', 'the ending landed, but it was too long')).toBe('');
  });
});
