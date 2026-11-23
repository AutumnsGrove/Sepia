/**
 * The search screen's own small state: turning a snippet into plain text and marked runs.
 *
 * A snippet arrives with the matched words wrapped in U+E000 and U+E001. They are private-use characters rather than
 * HTML on purpose: a review of "<script>" is her own text, so the browser must escape it, and only the markers decide
 * what is highlighted (api/openapi.yaml, SearchHit.snippet).
 */

/** The two markers the server wraps a matched word in. */
export const SNIPPET_OPEN = '\uE000';
export const SNIPPET_CLOSE = '\uE001';

/** One run of a snippet: either her text or a matched word. */
export type SnippetSegment = { text: string; marked: boolean };

/**
 * splitSnippet turns a snippet into runs, so the page can render the marked ones as <mark> and leave the rest escaped.
 * An unbalanced marker is treated as ordinary text: a broken snippet should read badly, never swallow the rest of the
 * row.
 */
export function splitSnippet(snippet: string): SnippetSegment[] {
  if (!snippet) return [];
  // A marker without its partner is not a marker. Falling back to the whole snippet as text keeps the row readable;
  // dropping it would silently delete the rest of her sentence.
  const opens = snippet.split(SNIPPET_OPEN).length - 1;
  const closes = snippet.split(SNIPPET_CLOSE).length - 1;
  if (opens !== closes) return [{ text: snippet, marked: false }];
  const segments: SnippetSegment[] = [];
  let marked = false;
  let current = '';
  for (const char of snippet) {
    if (char === SNIPPET_OPEN && !marked) {
      if (current) segments.push({ text: current, marked });
      current = '';
      marked = true;
      continue;
    }
    if (char === SNIPPET_CLOSE && marked) {
      if (current) segments.push({ text: current, marked: true });
      current = '';
      marked = false;
      continue;
    }
    current += char;
  }
  if (current) segments.push({ text: current, marked });
  return segments;
}

/**
 * snippetText is the snippet as plain words, which is what a label, a title or a test needs. It is also the fallback
 * path: a snippet with no markers reads exactly as it arrived.
 */
export function snippetText(snippet: string): string {
  return splitSnippet(snippet)
    .map((segment) => segment.text)
    .join('');
}

/**
 * mentionLine is the sentence above the results: "3 entries mention "slow"". It says entries rather than results
 * because a search reads entries, and it quotes the words she typed rather than the words that were searched, so a
 * phrase with punctuation says what she typed.
 */
export function mentionLine(total: number, query: string): string {
  const words = query.trim();
  const noun = total === 1 ? 'entry' : 'entries';
  const verb = total === 1 ? 'mentions' : 'mention';
  return `${total} ${noun} ${verb} “${words}”`;
}
