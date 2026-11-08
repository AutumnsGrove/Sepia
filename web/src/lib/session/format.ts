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
