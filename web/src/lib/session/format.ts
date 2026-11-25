/**
 * The small pieces of formatting the card and the why sheet share. Each one is pure and tested here rather than
 * in a component, because "4.50" instead of "4.5" is exactly the kind of drift a screenshot catches late.
 */

/** Quarters of a star (1 to 20) as the number she sees: 18 is 4.5, 20 is 5, and nothing is "Not rated". */
export function ratingText(quarters: number): string {
  if (!Number.isFinite(quarters) || quarters <= 0) return 'Not rated';
  const stars = quarters / 4;
  return stars.toFixed(2).replace(/\.?0+$/, '');
}

/** Money as it is shown: four decimals under a cent, two above, so a small call is not rounded to nothing. */
export function usdText(amount: number): string {
  if (!Number.isFinite(amount)) return '$0.00';
  const digits = amount > 0 && amount < 0.01 ? 4 : 2;
  return `$${amount.toFixed(digits)}`;
}

/**
 * A date from the card, in the mockups' shape: "Fri 30 Oct". A date with no time is read at noon UTC so the
 * label cannot slide a day in a timezone behind or ahead of the phone.
 */
export function dateText(iso: string | null | undefined): string {
  if (!iso) return 'Not set';
  const date = new Date(`${iso}T12:00:00Z`);
  if (Number.isNaN(date.getTime())) return iso;
  return new Intl.DateTimeFormat('en-GB', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(date);
}

/** The medium in the card's meta line: "Book", "Film", "TV". */
export function mediumText(medium: string | undefined): string {
  switch (medium) {
    case 'book':
      return 'Book';
    case 'film':
      return 'Film';
    case 'tv':
      return 'TV';
    case 'game':
      return 'Game';
    case 'album':
      return 'Album';
    case 'podcast':
      return 'Podcast';
    case 'theatre':
      return 'Theatre';
    default:
      return '';
  }
}

/**
 * The formats the card offers, in the order it offers them. The picker and the label read this one list, so a format
 * cannot be offered but unnameable, which is how "audiobook" once showed up raw in the date stamp.
 */
export const FORMATS = [
  ['cinema', 'Cinema'],
  ['streaming', 'Streaming'],
  ['broadcast', 'TV broadcast'],
  ['audiobook', 'Audiobook'],
  ['hardback', 'Hardback'],
  ['paperback', 'Paperback'],
  ['ebook', 'Ebook'],
  ['physical', 'Physical'],
  ['live', 'Live'],
] as const;

/** A format's plain name, or nothing for the empty and unknown values. */
export function formatText(format: string | undefined): string {
  return FORMATS.find(([value]) => value === format)?.[1] ?? '';
}

/**
 * A provider id as a name. The id is what the database stores, so "openlibrary" on a card or a page reads like a
 * bug; the metadata sources have had plain names since the mockups. The names mirror the ones the Go activity copy uses
 * (internal/agent/words.go), so one provider is called the same thing in a transcript and on an entry.
 */
export function providerText(provider: string | undefined): string {
  switch (provider) {
    case 'openlibrary':
      return 'Open Library';
    case 'tmdb':
      return 'TMDB';
    case 'polaris':
      return 'Polaris';
    // A work she wrote in herself, kept as a provider so the record can say where its facts came from.
    case 'manual':
      return 'Your own details';
    // An id this build has no name for says nothing rather than printing itself on the page.
    default:
      return '';
  }
}

/**
 * The verdict is a one-line pull quote, and the body is the same words at their positions in her message. When the body
 * already holds the verdict there is nothing to pull out, so it is not printed a second time above the body.
 *
 * It is matched as a sequence of words, not a substring: "fine" is not contained in "defined", and a real verdict
 * must not disappear because a longer word happens to hold its letters. Case and punctuation are ignored on both sides.
 */
export function verdictText(verdict: string | undefined, body: string | undefined): string {
  const line = (verdict ?? '').trim();
  if (line === '') return '';
  const wanted = wordsOf(line);
  if (wanted.length === 0) return line;
  const written = wordsOf(body ?? '');
  for (let start = 0; start + wanted.length <= written.length; start++) {
    if (wanted.every((word, offset) => written[start + offset] === word)) return '';
  }
  return line;
}

/** The letters and digits of a text, folded to lower case, which is the only comparison the verdict needs. */
function wordsOf(text: string): string[] {
  return text.toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
}
