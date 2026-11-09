import type { Entry } from '../api/client';
import type { Draft } from '../contract/events';
import type { ConnectionState } from '../socket/machine';
import { emptySession, type SessionState } from '../state/session';
import type { PendingMessage } from './screen';

/**
 * The invented review the mockups and the dev stack both use. Every screen state in the gallery is built from it,
 * so a baseline never depends on a real review or a live service (docs/BUILD.md invariant 10).
 */
const message =
  'finished the lantern keeper last night, 4.5, the ending landed but it was too long';

export const work = {
  id: 'w_lantern',
  medium: 'book' as const,
  title: 'The Lantern Keeper',
  year: 1999,
  creators: [{ role: 'author', name: 'Ada Voss' }],
  verified: true,
  primary_provider: 'openlibrary',
  primary_id: 'OL1W',
};

function draft(patch: Partial<Draft> = {}): Draft {
  return {
    version: 4,
    work,
    rating: { quarters: 18, scale: 'quarter_stars_20', favourite: false },
    verdict: 'the ending landed',
    body: 'the ending landed but it was too long',
    body_origin: 'extracted',
    format: 'paperback',
    finished_on: '2026-10-30',
    blocks: {
      work: 'verified',
      rating: 'set',
      body: 'extracted',
      facts: 'parsed',
      answers: 'answered',
    },
    left_out: [{ id: 'seg_0_45', text: 'finished the lantern keeper last night, 4.5, ' }],
    answers: [
      {
        question_id: 'setting',
        category: 'Setting and world',
        prompt: '',
        answer_type: 'tags',
        values: ['lived-in', 'immersive'],
      },
      {
        question_id: 'book_ending',
        prompt: 'How did the ending land?',
        answer_type: 'choice',
        values: ['It landed'],
      },
      {
        question_id: 'book_recommend',
        prompt: 'Would you recommend it?',
        answer_type: 'choice',
        values: [],
        skipped: true,
      },
    ],
    tags: [
      { name: 'lighthouses', source: 'genre' },
      { name: 'memory', source: 'mine' },
    ],
    why: {
      checks: [
        { check: 'intent', option: 'log_review', probability: 0.94, fired: true },
        { check: 'medium', option: 'book', probability: 0.55, fired: true },
        { check: 'medium', option: 'film', probability: 0.4, fired: false },
        { check: 'mentions_story', option: 'yes', probability: 0.91, fired: true },
        { check: 'mentions_pacing', option: 'yes', probability: 0.87, fired: true },
        { check: 'format', option: 'not_stated', probability: 0.09, fired: false },
      ],
      tools: [
        { tool: 'media_lookup', ok: true, summary: 'Chose The Lantern Keeper' },
        {
          tool: 'merge_candidates',
          ok: true,
          summary: 'Merged The Lantern Keeper from openlibrary',
          sources: [{ url: 'https://openlibrary.example/OL1W', title: 'openlibrary' }],
        },
      ],
      preread_cost: { total_usd: 0.0004, covered_usd: 0 },
      turn_cost: { total_usd: 0.0021, covered_usd: 0.0021 },
    },
    can_confirm: true,
    missing: [],
    ...patch,
  };
}

function session(patch: Partial<SessionState> = {}): SessionState {
  return {
    ...emptySession('s_lantern'),
    nextOrder: 10,
    ...patch,
  };
}

function you(order: number, text = message): SessionState['blocks'][number] {
  return {
    kind: 'user',
    id: `m-${order}`,
    clientMsgID: `c-${order}`,
    text,
    at: '2026-11-09T12:00:00Z',
    order,
  };
}

function act(
  order: number,
  label: string,
  patch: Partial<SessionState['activity'][number]> = {},
): SessionState['activity'][number] {
  return {
    id: `a-${order}`,
    label,
    sources: [],
    live: false,
    at: '2026-11-09T12:00:00Z',
    order,
    ...patch,
  };
}

const mediumPayload = {
  options: [
    { value: 'book', label: 'The book', hint: 'novel' },
    { value: 'film', label: 'A film', hint: 'movie' },
    { value: 'other', label: 'Something else' },
  ],
};

const candidatePayload = {
  candidates: [
    {
      id: 'openlibrary|OL1W',
      title: 'The Lantern Keeper',
      year: 1999,
      creator: 'Ada Voss',
      medium: 'book',
    },
    {
      id: 'openlibrary|OL2W',
      title: 'Lantern Keepers at Sea',
      year: 2004,
      creator: 'Tomas Reed',
      medium: 'book',
    },
  ],
  none_label: 'None of these',
};

const describePayload = {
  categories: [
    {
      id: 'setting',
      label: 'Setting',
      covered: true,
      tags: [
        { value: 'lived-in', label: 'lived-in', tone: 'positive' },
        { value: 'immersive', label: 'immersive', tone: 'positive' },
        { value: 'bleak', label: 'bleak', tone: 'negative' },
      ],
      hints: ['lived-in'],
    },
    {
      id: 'plot',
      label: 'Plot',
      covered: true,
      tags: [
        { value: 'drags in the middle', label: 'drags in the middle', tone: 'negative' },
        { value: 'slow burn', label: 'slow burn', tone: 'mixed' },
        { value: 'satisfying conclusion', label: 'satisfying conclusion', tone: 'positive' },
      ],
      hints: ['drags in the middle'],
    },
    {
      id: 'feel',
      label: 'Feel',
      tags: [
        { value: 'moving', label: 'moving', tone: 'positive' },
        { value: 'haunting', label: 'haunting', tone: 'positive' },
        { value: 'left me cold', label: 'left me cold', tone: 'negative' },
      ],
    },
  ],
};

const lookupPayload = {
  service: 'Open Library',
  reason: 'It did not answer in time.',
  can_ask_polaris: false,
};

const savedEntry: Entry = {
  id: 'e_lantern',
  work,
  rating_q: 18,
  rating_scale: 'quarter_stars_20',
  favourite: false,
  verdict: 'the ending landed',
  body: 'the ending landed but it was too long',
  body_origin: 'extracted',
  status: 'saved',
  format: 'paperback',
  finished_on: '2026-10-30',
  tags: [
    { name: 'lighthouses', source: 'genre' },
    { name: 'memory', source: 'mine' },
  ],
  created_at: '2026-11-09T12:00:00Z',
  updated_at: '2026-11-09T12:00:00Z',
};

export interface ScreenFixture {
  session: SessionState;
  connection: ConnectionState;
  pending: PendingMessage[];
  savedEntry: Entry | null;
  versions: number;
  caughtUp: boolean;
  problem: string;
}

/** One gallery state per mockup frame on pages 01, 02, 06 and 07 (the import frames belong to S18). */
export const screenNames = [
  'screen-split',
  'screen-candidates',
  'screen-describe',
  'screen-draft',
  'screen-saved',
  'screen-unverified',
  'screen-fidelity',
  'screen-editing',
  'screen-missing-rating',
  'screen-offline',
  'screen-caught-up',
  'screen-working',
  'screen-queued',
  'screen-error',
  'screen-preread-down',
  'screen-lookup-failed',
  'screen-hostile',
  'screen-stale',
] as const;

export type ScreenName = (typeof screenNames)[number];

export function screenFixture(name: string): ScreenFixture {
  const base: ScreenFixture = {
    session: session(),
    connection: 'open',
    pending: [],
    savedEntry: null,
    versions: 0,
    caughtUp: false,
    problem: '',
  };
  switch (name) {
    case 'screen-split':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Read your message')],
          interactions: [{ id: 'i1', kind: 'choose_medium', payload: mediumPayload, order: 3 }],
        }),
      };
    case 'screen-candidates':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Read as a book'), act(3, 'Looked up Open Library')],
          interactions: [
            { id: 'i1', kind: 'choose_candidate', payload: candidatePayload, order: 4 },
          ],
        }),
      };
    case 'screen-describe':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Matched The Lantern Keeper (1999)')],
          interactions: [{ id: 'i1', kind: 'describe', payload: describePayload, order: 4 }],
        }),
      };
    case 'screen-draft':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Matched The Lantern Keeper (1999)'), act(3, 'Read the ending')],
          draft: draft(),
          draftVersion: 4,
        }),
      };
    case 'screen-saved':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          draft: draft(),
          draftVersion: 5,
          status: 'saved',
          savedEntryID: 'e_lantern',
        }),
      };
    case 'screen-unverified':
      return {
        ...base,
        session: session({
          blocks: [
            you(
              1,
              'saw the salt road at the marlowe last night, 4.25, tiny cast and it still filled the room',
            ),
          ],
          draft: draft({
            work: {
              title: 'The Salt Road',
              year: 2026,
              creators: [{ role: 'playwright', name: 'Marlowe Players' }],
              medium: 'theatre',
              verified: false,
            },
            blocks: {
              work: 'unverified',
              rating: 'set',
              body: 'typed',
              facts: 'empty',
              answers: 'empty',
            },
            rating: { quarters: 17, scale: 'quarter_stars_20', favourite: false },
            format: '',
            finished_on: undefined,
            body: 'tiny cast and it still filled the room',
            body_origin: 'typed',
            verdict: '',
            left_out: [],
            answers: [],
            tags: [],
            unverified: [
              {
                id: 'web|salt-road',
                field: 'work',
                value: 'The Salt Road (2026)',
                source_url: 'marlowe-players.example/the-salt-road',
                snippet: 'The Salt Road. A new play. Marlowe Players, spring 2026.',
              },
            ],
            can_confirm: true,
            missing: [],
          }),
          draftVersion: 2,
        }),
      };
    case 'screen-fidelity':
      return {
        ...base,
        session: session({
          blocks: [
            you(
              1,
              'just got home from the lantern keeper, honestly blown away, 5/5 and the ending wrecked me, though the middle was muddy',
            ),
          ],
          draft: draft({
            rating: { quarters: 20, scale: 'quarter_stars_20', favourite: true },
            body: 'honestly blown away, 5/5 and the ending wrecked me',
            verdict: '',
            fidelity: {
              status: 'drops_opinion',
              warning: 'Something you said may be missing. Have a look at what was left out.',
            },
            left_out: [
              { id: 'seg_0_30', text: 'just got home from the lantern keeper, ' },
              { id: 'seg_70_100', text: 'though the middle was muddy' },
            ],
          }),
          draftVersion: 6,
        }),
      };
    case 'screen-editing':
      return {
        ...base,
        savedEntry,
        versions: 2,
        session: session({
          blocks: [
            you(1, 'on a second read the lantern keeper felt slower but the end stayed with me'),
          ],
          draft: draft({
            rating: { quarters: 19, scale: 'quarter_stars_20', favourite: false },
            body: 'on a second read the lantern keeper felt slower but the end stayed with me',
            body_origin: 'extracted',
          }),
          draftVersion: 7,
        }),
      };
    case 'screen-missing-rating':
      return {
        ...base,
        session: session({
          blocks: [you(1, 'just finished the quarry line season 3, so good, that finale though')],
          activity: [act(2, 'Matched The Quarry Line (2016)')],
          draft: draft({
            work: {
              id: 'w_quarry',
              medium: 'tv',
              title: 'The Quarry Line',
              year: 2016,
              creators: [{ role: 'creator', name: 'Nell Farrier' }],
              verified: true,
              primary_provider: 'tmdb',
            },
            rating: { quarters: 0, scale: 'quarter_stars_20', favourite: false },
            body: 'so good, that finale though',
            blocks: {
              work: 'verified',
              rating: 'empty',
              body: 'extracted',
              facts: 'parsed',
              answers: 'empty',
            },
            verdict: '',
            left_out: [],
            answers: [],
            tags: [],
            can_confirm: false,
            missing: ['rating'],
          }),
          draftVersion: 2,
        }),
      };
    case 'screen-offline':
      return {
        ...base,
        connection: 'reconnecting',
        pending: [
          {
            id: 'p1',
            text: 'just finished the lantern keeper, honestly one of the best things i have read, 4.75',
            queued: false,
          },
        ],
        session: session({ blocks: [] }),
      };
    case 'screen-caught-up':
      return {
        ...base,
        caughtUp: true,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Read as a book')],
          interactions: [
            { id: 'i1', kind: 'choose_candidate', payload: candidatePayload, order: 3 },
          ],
        }),
      };
    case 'screen-working':
      return {
        ...base,
        session: session({
          blocks: [
            you(1, 'watched the lantern keeper at the cinema, 5/5, the sound design carried it'),
          ],
          activity: [
            act(2, 'Read as a film'),
            act(3, 'Matched The Lantern Keeper (2019)'),
            act(4, 'Pulling details from TMDB', { live: true }),
          ],
        }),
      };
    case 'screen-queued':
      return {
        ...base,
        pending: [{ id: 'p1', text: 'oh and it was the imax screening', queued: true }],
        session: session({
          blocks: [
            you(1, 'watched the lantern keeper at the cinema, 5/5, the sound design carried it'),
          ],
          activity: [
            act(2, 'Matched The Lantern Keeper (2019)'),
            act(3, 'Pulling details from TMDB', { live: true }),
          ],
          queue: 1,
        }),
      };
    case 'screen-error':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Matched The Lantern Keeper (1999)')],
          error: {
            code: 'unavailable',
            message: 'Sepia cannot reach that right now.',
            retryable: true,
          },
        }),
      };
    case 'screen-preread-down':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          notices: [
            {
              level: 'info',
              code: 'preread_unavailable',
              text: 'The quick pre-read was not available for this message, so it went straight to the agent. You may see a few more questions than usual.',
              at: '2026-11-09T12:00:00Z',
              order: 2,
            },
          ],
          activity: [act(3, 'Read as a book')],
          interactions: [
            { id: 'i1', kind: 'choose_candidate', payload: candidatePayload, order: 4 },
          ],
        }),
      };
    case 'screen-hostile':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          draft: draft({
            work: {
              ...work,
              title: '<img src=x onerror="window.__xss=1">The Lantern Keeper',
              creators: [{ role: 'author', name: '<script>window.__xss=2</script>' }],
            },
            verdict: '<b>not bold</b>',
          }),
          draftVersion: 3,
        }),
      };
    case 'screen-stale':
      return {
        ...base,
        problem: 'The card changed while you were editing. Here it is again.',
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Matched The Lantern Keeper (1999)')],
          draft: draft({
            version: 6,
            rating: { quarters: 19, scale: 'quarter_stars_20', favourite: false },
          }),
          draftVersion: 6,
        }),
      };
    case 'screen-lookup-failed':
      return {
        ...base,
        session: session({
          blocks: [you(1)],
          activity: [act(2, 'Read as a book')],
          interactions: [{ id: 'i1', kind: 'lookup_failed', payload: lookupPayload, order: 3 }],
        }),
      };
    default:
      return base;
  }
}

/** What a fixture screen does when a control is pressed: nothing, because a baseline is not a behaviour test. */
export const fixtureHandlers = {
  onsend: () => {},
  onanswer: () => {},
  onedit: () => {},
  oninclude: () => {},
  onaccept: () => {},
  onacceptall: () => {},
  onconfirm: () => {},
  ondiscard: () => {},
  onstop: () => {},
  onretry: () => {},
  oncancel: () => {},
  onback: () => {},
  oneditSaved: () => {},
  onlogAnother: () => {},
  onmore: () => {},
  onversions: () => {},
};
