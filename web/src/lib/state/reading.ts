import type { Entry, Revision } from '../api/client';
import { mediumText, ratingText } from '../session/format';

/**
 * The reading screens' derived state. Each function is pure and tested here rather than in a component: the month a
 * row lands in, the sentence that says why a draft is waiting, and what changed between two versions are exactly the
 * things a screenshot catches late (docs/TESTING.md 6).
 */

// --- lists ---------------------------------------------------------------------------------------------

export interface MonthGroup {
  /** The sort key: "2026-10", or "" for entries with no finish date. */
  key: string;
  /** The heading: "October 2026", or "No date". */
  label: string;
  items: Entry[];
}

const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

/**
 * The library grouped by the month she finished each thing, in the order the server sent them. Entries saved without
 * a finish date are not dropped: they are a group of their own at the end, which is where the server's order puts
 * them.
 */
export function groupByMonth(entries: Entry[]): MonthGroup[] {
  const groups: MonthGroup[] = [];
  const byKey = new Map<string, MonthGroup>();
  for (const entry of entries) {
    const key = (entry.finished_on ?? '').slice(0, 7);
    let group = byKey.get(key);
    if (!group) {
      group = { key, label: key === '' ? 'No date' : monthLabel(key), items: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.items.push(entry);
  }
  return groups;
}

/** "2026-10" as the heading she reads. A month outside the calendar cannot happen, but it does not throw either. */
export function monthLabel(key: string): string {
  const [year, month] = key.split('-');
  const index = Number(month) - 1;
  const name = MONTHS[index];
  if (!name) return key;
  return `${name} ${year}`;
}

/** "Book · 30 Oct", with the year only when it is not this one: the month heading already says it for this year. */
export function entryMeta(entry: Entry, thisYear: number): string {
  const medium = mediumText(entry.work.medium);
  const date = shortDate(entry.finished_on, thisYear);
  if (!medium) return date;
  return date ? `${medium} · ${date}` : medium;
}

/** The day and month a row can show, or the year too when it is not the current one. */
export function shortDate(iso: string | null | undefined, thisYear: number): string {
  if (!iso) return '';
  const date = new Date(`${iso}T12:00:00Z`);
  if (Number.isNaN(date.getTime())) return iso;
  const day = date.getUTCDate();
  const month = MONTHS[date.getUTCMonth()]?.slice(0, 3) ?? '';
  if (date.getUTCFullYear() === thisYear) return `${day} ${month}`;
  return `${day} ${month} ${date.getUTCFullYear()}`;
}

/** The atom under a Deleted row: when it happened, and nothing else, because nothing else is stored. */
export function deletedMeta(entry: Entry, thisYear: number): string {
  if (!entry.deleted_at) return 'Deleted';
  const date = new Date(entry.deleted_at);
  if (Number.isNaN(date.getTime())) return 'Deleted';
  const stamp = new Intl.DateTimeFormat('en-GB', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    year: date.getUTCFullYear() === thisYear ? undefined : 'numeric',
    timeZone: 'UTC',
  }).format(date);
  return `Deleted ${stamp}`;
}

/** The entries of a work other than the one being read, which is the timeline under a saved entry. */
export function otherEntries(entries: Entry[], currentID: string): Entry[] {
  return entries.filter((entry) => entry.id !== currentID);
}

/** A count in the top bar or on a settings row. Above the whole page it was read from, it says "99+" rather than a
 * number it does not know. */
export function countText(count: number, limit = 100): string {
  if (count >= limit) return '99+';
  return String(count);
}

// --- drafts --------------------------------------------------------------------------------------------

const WAITING: Record<string, string> = {
  pick_work: 'Waiting for you to pick which one',
  needs_rating: 'Needs a rating',
  needs_body: 'Needs your words',
  accept_fields: 'Web fields to accept',
  needs_date: 'Needs a date',
  ready: 'Ready to save',
};

/** What a draft is waiting for, in the contract's words rather than its codes. A reason Sepia does not know is
 * shown as the plain "waiting" it is, never as a blank row. */
export function waitingText(reason: string | null | undefined): string {
  if (!reason) return 'Waiting for you';
  return WAITING[reason] ?? 'Waiting for you';
}

/** How long ago she touched a draft: the mockups' shape, which is coarser as it gets older. */
export function ageText(iso: string | null | undefined, now: Date): string {
  if (!iso) return '';
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return '';
  const minutes = Math.floor((now.getTime() - at.getTime()) / 60000);
  if (minutes < 2) return 'Just now';
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.floor(hours / 24);
  if (days === 1) return 'Yesterday';
  if (days < 30) return `${days} days ago`;
  return new Intl.DateTimeFormat('en-GB', {
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(at);
}

// --- the entry page ------------------------------------------------------------------------------------

/** The category labels the Describe step uses. The chips themselves are a closed vocabulary in config/; this is only
 * the label over them, and an unknown category falls back to its key rather than vanishing. */
const CATEGORY_LABELS: Record<string, string> = {
  characters: 'Characters',
  plot: 'Plot',
  setting: 'Setting and world',
  writing: 'Writing style',
  story: 'Story',
  performances: 'Performances',
  visuals: 'Visuals',
  sound: 'Sound and music',
  pacing: 'Pacing',
  feel: 'Feel',
  content_warnings: 'Content warnings',
};

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category.replace(/_/g, ' ');
}

export interface DescribeGroup {
  category: string;
  label: string;
  values: string[];
}

interface Answer {
  question_id: string;
  category?: string;
  prompt: string;
  answer_type: 'tags' | 'choice' | 'short_text';
  values: string[];
  skipped?: boolean;
}

/** The descriptor chips, grouped by category, in the order they were answered. Content warnings are their own line
 * on the card, so they are not part of this list. */
export function describeGroups(answers: Answer[]): DescribeGroup[] {
  const groups: DescribeGroup[] = [];
  const byCategory = new Map<string, DescribeGroup>();
  for (const answer of answers) {
    if (answer.answer_type !== 'tags' || answer.skipped) continue;
    if (!answer.category || answer.category === 'content_warnings') continue;
    if (answer.values.length === 0) continue;
    let group = byCategory.get(answer.category);
    if (!group) {
      group = { category: answer.category, label: categoryLabel(answer.category), values: [] };
      byCategory.set(answer.category, group);
      groups.push(group);
    }
    for (const value of answer.values) {
      if (!group.values.includes(value)) group.values.push(value);
    }
  }
  return groups;
}

/** The single questions and what she said, skipped ones left out: a row that says "Skipped" is not an answer. */
export function questionAnswers(answers: Answer[]): { prompt: string; values: string[] }[] {
  return answers
    .filter(
      (answer) => answer.answer_type !== 'tags' && !answer.skipped && answer.values.length > 0,
    )
    .map((answer) => ({ prompt: answer.prompt, values: answer.values }));
}

/** The content warnings line, as the mockup has it. */
export function warningsOf(answers: Answer[]): string[] {
  const out: string[] = [];
  for (const answer of answers) {
    if (answer.category !== 'content_warnings' || answer.skipped) continue;
    for (const value of answer.values) {
      if (!out.includes(value)) out.push(value);
    }
  }
  return out;
}

// --- versions ------------------------------------------------------------------------------------------

export interface VersionRow {
  /** 1 is the first save. */
  number: number;
  /** The instant the version was confirmed, for the heading. */
  at: string;
  /** The card as it was then. */
  entry: Entry;
  /** What changed in this version, in one sentence. */
  change: string;
  current: boolean;
}

/**
 * The versions screen's rows: the entry as it is now, then every version it replaced, newest first. Each row says what
 * that version changed *from the one before it*, because that is the question a history answers.
 */
export function versionRows(current: Entry, revisions: Revision[]): VersionRow[] {
  const rows: VersionRow[] = [];
  const ordered = [...revisions].sort((a, b) => b.revision - a.revision);
  const total = ordered.length + 1;
  rows.push({
    number: total,
    at: current.updated_at,
    entry: current,
    change: ordered[0] ? changeBetween(ordered[0].snapshot, current) : 'First saved.',
    current: true,
  });
  ordered.forEach((revision, index) => {
    const older = ordered[index + 1];
    rows.push({
      number: revision.revision,
      at: revision.created_at,
      entry: revision.snapshot,
      change: older ? changeBetween(older.snapshot, revision.snapshot) : 'First saved.',
      current: false,
    });
  });
  return rows;
}

/** One sentence for what a version changed, in the card's own words. */
export function changeBetween(before: Entry, after: Entry): string {
  const parts: string[] = [];
  if (before.rating_q !== after.rating_q) {
    parts.push(`Rating ${ratingText(before.rating_q)} to ${ratingText(after.rating_q)}.`);
  }
  if (before.body !== after.body) {
    parts.push(
      after.body.startsWith(before.body)
        ? 'Words extended.'
        : before.body.startsWith(after.body)
          ? 'Words shortened.'
          : 'Words changed.',
    );
  }
  if ((before.format ?? '') !== (after.format ?? '')) {
    parts.push(after.format ? `Format set to ${titleCase(after.format)}.` : 'Format cleared.');
  }
  if ((before.finished_on ?? '') !== (after.finished_on ?? '')) {
    parts.push(after.finished_on ? 'Finish date changed.' : 'Finish date cleared.');
  }
  if ((before.verdict ?? '') !== (after.verdict ?? '')) {
    parts.push(after.verdict ? 'Verdict added.' : 'Verdict cleared.');
  }
  if (Boolean(before.favourite) !== Boolean(after.favourite)) {
    parts.push(after.favourite ? 'Marked as a favourite.' : 'No longer a favourite.');
  }
  return parts.length === 0 ? 'No visible change.' : parts.join(' ');
}

function titleCase(value: string): string {
  return value.length === 0 ? value : value[0].toUpperCase() + value.slice(1);
}
