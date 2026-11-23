import { describe, expect, it } from 'vitest';
import { mentionLine, snippetText, splitSnippet, SNIPPET_CLOSE, SNIPPET_OPEN } from './search';

// The snippet markers are the only thing standing between her own review text and a rendered page, so they are tested
// on the shapes a real snippet takes and on the ones a broken one takes.

describe('splitSnippet', () => {
  it('marks the matched word and leaves the rest alone', () => {
    const segments = splitSnippet(`a bit too ${SNIPPET_OPEN}slow${SNIPPET_CLOSE} in places`);
    expect(segments).toEqual([
      { text: 'a bit too ', marked: false },
      { text: 'slow', marked: true },
      { text: ' in places', marked: false },
    ]);
  });

  it('marks every match in one snippet', () => {
    const segments = splitSnippet(
      `${SNIPPET_OPEN}slow${SNIPPET_CLOSE} then ${SNIPPET_OPEN}slow${SNIPPET_CLOSE}`,
    );
    expect(segments.filter((segment) => segment.marked).map((segment) => segment.text)).toEqual([
      'slow',
      'slow',
    ]);
  });

  it('leaves HTML as text, so a review about a tag is a sentence', () => {
    const segments = splitSnippet(
      `<script>alert(1)</script> and ${SNIPPET_OPEN}slow${SNIPPET_CLOSE}`,
    );
    expect(snippetText(segments.map((segment) => segment.text).join(''))).toBe(
      '<script>alert(1)</script> and slow',
    );
    expect(segments.some((segment) => segment.marked)).toBe(true);
  });

  it('treats an unbalanced marker as ordinary text rather than swallowing the row', () => {
    const segments = splitSnippet(`slow ${SNIPPET_OPEN}all the rest`);
    expect(snippetText(`${segments.map((segment) => segment.text).join('')}`)).toBe(
      `slow ${SNIPPET_OPEN}all the rest`,
    );
  });

  it('is empty for an empty snippet, which is what a title-only match sends', () => {
    expect(splitSnippet('')).toEqual([]);
  });
});

describe('mentionLine', () => {
  it('says how many entries matched, and quotes what she typed', () => {
    expect(mentionLine(3, 'slow')).toBe('3 entries mention “slow”');
    expect(mentionLine(1, ' slow ')).toBe('1 entry mentions “slow”');
    expect(mentionLine(0, 'zwift')).toBe('0 entries mention “zwift”');
  });
});
