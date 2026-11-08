import type { Draft, ServerEvent } from '../contract/events';

/**
 * The session, derived from the events on the socket and nothing else. A component reads this state and never
 * mutates it: every change arrives as an event, and the reducer is the only thing that writes (S10.5).
 *
 * The reducer is a pure function so it can be tested against the golden examples in `api/examples/` without a
 * browser, a socket or a server. The reactive wrapper the screens use is a thin `$state` holder around it.
 */

export type SessionStatus = 'active' | 'saved' | 'discarded';

export type TranscriptBlock =
  | { kind: 'user'; id: string; clientMsgID: string; text: string; at: string; order: number }
  | { kind: 'assistant'; id: string; text: string; streaming: boolean; at: string; order: number };

/** A quiet line under the transcript: what the agent is doing, not what it said. */
export interface ActivityRow {
  id: string;
  label: string;
  tool?: string;
  ok?: boolean;
  summary?: string;
  sources: { url: string; title: string }[];
  live: boolean;
  at: string;
  /** Where this row sits in the one transcript order, beside blocks, notices and cards. */
  order: number;
}

export interface OpenInteraction {
  id: string;
  kind: string;
  payload: Record<string, unknown>;
  order: number;
}

export interface NoticeRow {
  level: string;
  code: string;
  text: string;
  at: string;
  order: number;
}

export interface SessionCost {
  /** What the session consumed, including anything a plan covered. */
  total: number;
  /** The part of `total` a plan covered, never an addition to it. */
  covered: number;
}

export interface Preread {
  odds: Record<string, unknown>;
  fired: string[];
  cost: number | null;
}

export interface SessionError {
  code: string;
  message: string;
  retryable: boolean;
}

export interface SessionState {
  sessionID: string;
  status: SessionStatus;
  headSeq: number;
  fullState: boolean;
  /** The next free place in the transcript order. One counter for every kind of row, so nothing needs a clock. */
  nextOrder: number;
  blocks: TranscriptBlock[];
  activity: ActivityRow[];
  interactions: OpenInteraction[];
  draft: Draft | null;
  draftVersion: number;
  cost: SessionCost;
  preread: Preread | null;
  notices: NoticeRow[];
  queue: number;
  error: SessionError | null;
  savedEntryID: string | null;
}

export interface Applied {
  state: SessionState;
  /** Set when the event is not one this client knows. The caller logs it; nothing else changes. */
  ignored?: string;
}

export function emptySession(sessionID = ''): SessionState {
  return {
    sessionID,
    status: 'active',
    headSeq: 0,
    fullState: false,
    nextOrder: 1,
    blocks: [],
    activity: [],
    interactions: [],
    draft: null,
    draftVersion: 0,
    cost: { total: 0, covered: 0 },
    preread: null,
    notices: [],
    queue: 0,
    error: null,
    savedEntryID: null,
  };
}

// The event variants, named so the reducer's casts are checked against the generated contract rather than a
// hand-written copy of it.
type EventOf<T extends string> = Extract<ServerEvent, { type?: T }>;
type DataOf<T extends string> = NonNullable<EventOf<T>['data']>;

function dataOf<T extends string>(event: ServerEvent): DataOf<T> {
  return (event.data ?? {}) as DataOf<T>;
}

/**
 * A hello is the state at a point in time. A reconnect resumes from `last_seq`, so the transcript is kept; a
 * `full_state` hello means the log was compacted past us, and the transcript is rebuilt from the session's own
 * messages fetched over REST rather than guessed from events that no longer exist.
 */
export function applyHello(state: SessionState, event: ServerEvent): SessionState {
  const data = dataOf<'hello'>(event);
  const session = (data.session ?? {}) as { id?: string; state?: SessionStatus };
  const next: SessionState = {
    ...state,
    sessionID: session.id ?? state.sessionID,
    status: session.state ?? 'active',
    headSeq: typeof data.head_seq === 'number' ? data.head_seq : state.headSeq,
    fullState: data.full_state === true,
    draft: (data.draft ?? null) as Draft | null,
    draftVersion: (data.draft as Draft | null)?.version ?? 0,
    interactions: ((data.open_interactions ?? []) as OpenInteraction[]).map(
      (interaction, index) => ({
        id: String((interaction as unknown as { interaction_id?: string }).interaction_id ?? ''),
        kind: interaction.kind,
        payload: (interaction.payload ?? {}) as Record<string, unknown>,
        // A hello's cards are already open when the page arrives; they sit at the end of the transcript it rebuilds.
        order: (data.full_state === true ? 1 : state.nextOrder) + index,
      }),
    ),
    // A hello is a new connection, not a new turn: an old error or queued count does not survive it.
    error: null,
  };
  if (data.full_state === true) {
    next.blocks = [];
    next.activity = [];
    next.notices = [];
    next.nextOrder = 1 + next.interactions.length;
  } else {
    next.nextOrder = state.nextOrder + next.interactions.length;
  }
  return next;
}

function withoutInteraction(state: SessionState, id: string): OpenInteraction[] {
  return state.interactions.filter((interaction) => interaction.id !== id);
}

/** The last activity row still waiting to end, for the tool it belongs to. */
function openActivityIndex(activity: ActivityRow[], tool: string): number {
  for (let i = activity.length - 1; i >= 0; i -= 1) {
    if (activity[i].tool === tool && activity[i].live) return i;
  }
  return -1;
}

/**
 * One event becomes one change. Every branch returns a new object; a caller may keep the old state and compare.
 * An unknown type is reported, never thrown: a newer server must not be able to break an older page.
 */
export function applyEvent(state: SessionState, event: ServerEvent): Applied {
  const at = event.ts;
  switch (event.type) {
    case 'hello':
      return { state: applyHello(state, event) };

    case 'user_message': {
      const data = dataOf<'user_message'>(event);
      if (state.blocks.some((block) => block.kind === 'user' && block.id === data.message_id)) {
        return { state };
      }
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          blocks: [
            ...state.blocks,
            {
              kind: 'user',
              id: data.message_id,
              clientMsgID: data.client_msg_id,
              text: data.text,
              at,
              order: state.nextOrder,
            },
          ],
        },
      };
    }

    case 'assistant_start': {
      const data = dataOf<'assistant_start'>(event);
      if (state.blocks.some((block) => block.id === data.message_id)) return { state };
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          blocks: [
            ...state.blocks,
            {
              kind: 'assistant',
              id: data.message_id,
              text: '',
              streaming: true,
              at,
              order: state.nextOrder,
            },
          ],
        },
      };
    }

    case 'text_delta': {
      const data = dataOf<'text_delta'>(event);
      return {
        state: {
          ...state,
          blocks: state.blocks.map((block) =>
            block.kind === 'assistant' && block.id === data.message_id
              ? { ...block, text: block.text + data.text }
              : block,
          ),
        },
      };
    }

    case 'assistant_end': {
      const data = dataOf<'assistant_end'>(event);
      return {
        state: {
          ...state,
          blocks: state.blocks.map((block) =>
            block.kind === 'assistant' && block.id === data.message_id
              ? { ...block, streaming: false }
              : block,
          ),
        },
      };
    }

    case 'activity': {
      const data = dataOf<'activity'>(event);
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          activity: [
            ...state.activity,
            {
              id: `a-${state.activity.length}`,
              label: data.label,
              sources: [],
              live: event.seq === null,
              at,
              order: state.nextOrder,
            },
          ],
        },
      };
    }

    case 'tool_start': {
      const data = dataOf<'tool_start'>(event);
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          activity: [
            ...state.activity,
            {
              id: `t-${state.activity.length}`,
              tool: data.tool,
              label: data.label,
              sources: [],
              live: true,
              at,
              order: state.nextOrder,
            },
          ],
        },
      };
    }

    case 'tool_end': {
      const data = dataOf<'tool_end'>(event);
      const row: ActivityRow = {
        id: `t-${state.activity.length}`,
        tool: data.tool,
        ok: data.ok,
        label: data.summary,
        summary: data.summary,
        sources: (data.sources ?? []).map((source) => ({
          url: String(source.url ?? ''),
          title: String(source.title ?? ''),
        })),
        live: false,
        at,
        order: state.nextOrder,
      };
      const index = openActivityIndex(state.activity, data.tool);
      if (index === -1) {
        return {
          state: { ...state, nextOrder: state.nextOrder + 1, activity: [...state.activity, row] },
        };
      }
      const activity = state.activity.slice();
      // The end of a tool lands where its start did: the row is the same moment in the transcript.
      activity[index] = {
        ...activity[index],
        ...row,
        id: activity[index].id,
        order: activity[index].order,
      };
      return { state: { ...state, activity } };
    }

    case 'interaction_request': {
      const data = dataOf<'interaction_request'>(event);
      if (state.interactions.some((interaction) => interaction.id === data.interaction_id)) {
        return { state };
      }
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          interactions: [
            ...state.interactions,
            {
              id: data.interaction_id,
              kind: data.kind,
              payload: (data.payload ?? {}) as Record<string, unknown>,
              order: state.nextOrder,
            },
          ],
        },
      };
    }

    case 'interaction_resolved': {
      const data = dataOf<'interaction_resolved'>(event);
      return { state: { ...state, interactions: withoutInteraction(state, data.interaction_id) } };
    }

    case 'draft_snapshot': {
      const data = dataOf<'draft_snapshot'>(event);
      // A snapshot that arrives late must not move the card backwards. The version is the ordering rule.
      if (data.version < state.draftVersion) return { state };
      return {
        state: { ...state, draft: data.draft as Draft, draftVersion: data.version },
      };
    }

    case 'saved': {
      const data = dataOf<'saved'>(event);
      return {
        state: {
          ...state,
          status: 'saved',
          savedEntryID: data.entry_id,
          interactions: [],
          queue: 0,
        },
      };
    }

    case 'discarded':
      return {
        state: {
          ...state,
          status: 'discarded',
          draft: null,
          interactions: [],
          queue: 0,
        },
      };

    case 'cost_update': {
      const data = dataOf<'cost_update'>(event);
      return {
        state: {
          ...state,
          cost: {
            total: data.session_total_usd,
            covered: data.session_covered_usd,
          },
        },
      };
    }

    case 'preread': {
      const data = dataOf<'preread'>(event);
      return {
        state: {
          ...state,
          preread: {
            odds: (data.odds ?? {}) as Record<string, unknown>,
            fired: data.fired ?? [],
            cost: typeof data.cost_usd === 'number' ? data.cost_usd : null,
          },
        },
      };
    }

    case 'notice': {
      const data = dataOf<'notice'>(event);
      return {
        state: {
          ...state,
          nextOrder: state.nextOrder + 1,
          notices: [
            ...state.notices,
            { level: data.level, code: data.code, text: data.text, at, order: state.nextOrder },
          ],
        },
      };
    }

    case 'error': {
      const data = dataOf<'error'>(event);
      return {
        state: {
          ...state,
          error: { code: data.code, message: data.message, retryable: data.retryable },
        },
      };
    }

    case 'queue_update': {
      const data = dataOf<'queue_update'>(event);
      return { state: { ...state, queue: data.queued } };
    }

    case 'ack':
      // The machine owns the outbox and surfaces the ack; the transcript does not change.
      return { state };

    default:
      // Unreachable through the generated union, reachable from a newer server: an old page must keep working.
      return { state, ignored: String((event as { type?: string }).type) };
  }
}

/** Fold a stream of events, which is what a replay is. */
export function applyAll(state: SessionState, events: ServerEvent[]): SessionState {
  return events.reduce((current, event) => applyEvent(current, event).state, state);
}
