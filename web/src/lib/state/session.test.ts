import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { checkServerEvent } from '../contract/validate';
import type { ServerEvent } from '../contract/events';
import { applyAll, applyEvent, applyHello, emptySession, type SessionState } from './session';
import { isWorking, timeline } from './timeline';

/**
 * The session reducer, fed the golden examples from api/examples/. Every fixture is first checked against the
 * contract, so a test cannot pass on a shape the real server would never send (S02's rule, kept here).
 */

const examplesDir = fileURLToPath(new URL('../../../../api/examples/', import.meta.url));

function fixture(name: string): ServerEvent {
  const raw = JSON.parse(readFileSync(`${examplesDir}${name}`, 'utf8'));
  const problem = checkServerEvent(raw);
  if (problem) throw new Error(`${name} is not in the contract: ${problem}`);
  return raw as ServerEvent;
}

function serverExamples(): string[] {
  return readdirSync(examplesDir).filter((name) => name.startsWith('server-'));
}

describe('every server example', () => {
  it('is in the contract and survives the reducer', () => {
    for (const name of serverExamples()) {
      const event = fixture(name);
      // The happy route for the whole vocabulary: no example throws, and only a type the reducer does not know
      // is reported as ignored.
      const applied = applyEvent(emptySession('s_8f2'), event);
      if (event.type !== 'hello') expect(applied.ignored, name).toBeUndefined();
      expect(applied.state.sessionID || event.session_id).toBe('s_8f2');
    }
  });
});

/**
 * One server event, in the contract's shape, so a test reads like the socket it stands in for. A null seq is a live
 * event, which is what an activity row is: it is drawn while the turn runs and never replayed.
 */
function chatEvent(
  type: string,
  data: Record<string, unknown>,
  seq: number | null = 1,
): ServerEvent {
  const event = { seq, type, session_id: 's_chat', ts: '2026-11-26T12:00:00Z', data };
  const problem = checkServerEvent(event);
  if (problem) throw new Error(`${type} is not in the contract: ${problem}`);
  return event as ServerEvent;
}

describe('a chat answer', () => {
  it('puts the entries a search found under the sentence that names them, after it', () => {
    const state = applyAll(emptySession('s_chat'), [
      chatEvent('activity', { label: 'Searching your library' }, null),
      chatEvent('assistant_start', { message_id: 'm1' }, 3),
      chatEvent('text_delta', { message_id: 'm1', text: 'One book. Piranesi.' }, 4),
      chatEvent('assistant_end', { message_id: 'm1' }, 5),
      chatEvent(
        'chat_results',
        {
          results: [
            {
              entry_id: 'e1',
              title: 'Piranesi',
              medium: 'book',
              completion: 'finished',
              rating_q: 19,
              finished_on: '2026-09-28',
            },
          ],
        },
        6,
      ),
    ]);
    const rows = timeline(state);
    const answerAt = rows.findIndex(
      (row) => row.kind === 'block' && row.block.kind === 'assistant',
    );
    const resultsAt = rows.findIndex((row) => row.kind === 'results');
    expect(answerAt).toBeGreaterThan(-1);
    expect(resultsAt).toBeGreaterThan(answerAt);
    const group = rows[resultsAt];
    if (group.kind !== 'results') throw new Error('not a results row');
    expect(group.rows).toHaveLength(1);
    expect(group.rows[0]).toMatchObject({
      entryID: 'e1',
      title: 'Piranesi',
      medium: 'book',
      ratingQ: 19,
      finishedOn: '2026-09-28',
      coverUrl: null,
    });
  });

  it("keeps two answers' rows apart, in the order the server ranked them", () => {
    const state = applyAll(emptySession('s_chat'), [
      chatEvent(
        'chat_results',
        {
          results: [
            { entry_id: 'e1', title: 'Piranesi', medium: 'book', completion: 'finished' },
            { entry_id: 'e2', title: 'The Salt Road', medium: 'book', completion: 'finished' },
          ],
        },
        1,
      ),
      chatEvent(
        'chat_results',
        {
          results: [
            { entry_id: 'e3', title: 'Grey Harbour', medium: 'tv', completion: 'finished' },
          ],
        },
        2,
      ),
    ]);
    const groups = timeline(state).filter((row) => row.kind === 'results');
    expect(groups).toHaveLength(2);
    if (groups[0].kind !== 'results' || groups[1].kind !== 'results')
      throw new Error('not results');
    expect(groups[0].rows.map((row) => row.entryID)).toEqual(['e1', 'e2']);
    expect(groups[1].rows.map((row) => row.entryID)).toEqual(['e3']);
  });
});

describe('the session reducer', () => {
  it('builds the transcript from a hello and a replay, in order', () => {
    let state = applyHello(emptySession(), fixture('server-hello.json'));
    expect(state.sessionID).toBe('s_8f2');
    expect(state.draftVersion).toBe(4);
    expect(state.draft?.work.title).toBe('Dune');

    state = applyEvent(state, fixture('server-assistant-start.json')).state;
    state = applyEvent(state, fixture('server-text-delta.json')).state;
    state = applyEvent(state, fixture('server-text-delta.json')).state;
    const assistant = state.blocks.find((block) => block.kind === 'assistant');
    expect(assistant?.kind === 'assistant' && assistant.text).toBe('Found it. Found it. ');

    state = applyEvent(state, fixture('server-assistant-end.json')).state;
    expect(state.blocks.every((block) => block.kind !== 'assistant' || !block.streaming)).toBe(
      true,
    );
  });

  it('pairs a tool with its end, and keeps its sources', () => {
    let state = applyEvent(emptySession(), fixture('server-tool-start.json')).state;
    expect(state.activity).toHaveLength(1);
    expect(state.activity[0].live).toBe(true);

    state = applyEvent(state, fixture('server-tool-end.json')).state;
    expect(state.activity).toHaveLength(1);
    expect(state.activity[0].live).toBe(false);
    expect(state.activity[0].ok).toBe(true);
    expect(state.activity[0].sources[0].url).toContain('openlibrary.org');
  });

  it('answers an interaction a hello carried, because the hello names it', () => {
    const state = applyHello(emptySession(), fixture('server-hello-open-interaction.json'));
    expect(state.interactions).toHaveLength(1);
    expect(state.interactions[0].id).toBe('i_7');
    expect(state.interactions[0].kind).toBe('choose_medium');
  });

  it('opens an interaction and closes it by id', () => {
    let state = applyEvent(emptySession(), fixture('server-interaction-choose-medium.json')).state;
    expect(state.interactions.map((i) => i.id)).toEqual(['i_1']);
    expect(state.interactions[0].kind).toBe('choose_medium');

    const resolved = {
      ...fixture('server-interaction-resolved.json'),
      data: { interaction_id: 'i_1', by: 'her' },
    } as ServerEvent;
    state = applyEvent(state, resolved).state;
    expect(state.interactions).toEqual([]);
  });

  it('keeps a late draft snapshot from moving the card backwards', () => {
    const state = applyEvent(emptySession(), fixture('server-draft-snapshot.json')).state;
    expect(state.draftVersion).toBe(4);
    const stale = {
      ...fixture('server-draft-snapshot.json'),
      data: { version: 2, draft: { ...(fixture('server-draft-snapshot.json').data as object) } },
    } as unknown as ServerEvent;
    const applied = applyEvent(state, stale);
    expect(applied.state.draftVersion).toBe(4);
  });

  it('treats a user message replayed twice as one message', () => {
    const event = fixture('server-user-message.json');
    const once = applyEvent(emptySession(), event).state;
    const twice = applyEvent(once, event).state;
    expect(twice.blocks).toHaveLength(1);
  });

  it('ends the session on saved and on discarded', () => {
    let state = applyEvent(emptySession(), fixture('server-hello.json')).state;
    state = applyEvent(state, fixture('server-saved.json')).state;
    expect(state.status).toBe('saved');
    expect(state.savedEntryID).toBe('e_41');

    let other = applyEvent(emptySession(), fixture('server-hello.json')).state;
    other = applyEvent(other, fixture('server-discarded.json')).state;
    expect(other.status).toBe('discarded');
    expect(other.draft).toBeNull();
  });

  it('carries the session cost with the covered part inside the total', () => {
    const state = applyEvent(emptySession(), fixture('server-cost-update.json')).state;
    expect(state.cost.total).toBeCloseTo(0.0031);
    expect(state.cost.covered).toBeCloseTo(0.001);
    expect(state.cost.covered).toBeLessThanOrEqual(state.cost.total);
  });

  it('reports an event type it does not know and changes nothing', () => {
    const before = emptySession('s1');
    const unknown = {
      seq: 9,
      type: 'from_the_future',
      session_id: 's1',
      ts: '2026-11-08T12:00:00Z',
      data: {},
    } as unknown as ServerEvent;
    const applied = applyEvent(before, unknown);
    expect(applied.ignored).toBe('from_the_future');
    expect(applied.state).toBe(before);
  });

  it('rebuilds from a full_state hello rather than keeping a transcript it cannot vouch for', () => {
    let state = applyEvent(emptySession(), fixture('server-user-message.json')).state;
    expect(state.blocks).toHaveLength(1);

    const compacted = {
      ...fixture('server-hello.json'),
      data: { ...(fixture('server-hello.json').data as object), full_state: true, head_seq: 99 },
    } as ServerEvent;
    state = applyHello(state, compacted);
    expect(state.fullState).toBe(true);
    expect(state.headSeq).toBe(99);
    expect(state.blocks).toEqual([]);
    expect(state.draftVersion).toBe(4);
  });

  it('folds a whole replay in one pass', () => {
    const events = [
      fixture('server-hello.json'),
      fixture('server-user-message.json'),
      fixture('server-assistant-start.json'),
      fixture('server-text-delta.json'),
      fixture('server-assistant-end.json'),
      fixture('server-draft-snapshot.json'),
    ];
    const state = applyAll(emptySession(), events);
    expect(state.blocks).toHaveLength(2);
    expect(state.draftVersion).toBe(4);
  });
});

describe('whether a turn is in flight', () => {
  it('is true for a live activity row, a streaming reply, and a queued message', () => {
    const idle = emptySession();
    expect(isWorking(idle)).toBe(false);

    const live = applyEvent(idle, fixture('server-activity.json')).state;
    expect(isWorking(live)).toBe(true);

    const streaming = applyEvent(idle, fixture('server-assistant-start.json')).state;
    expect(isWorking(streaming)).toBe(true);

    const ended = applyEvent(streaming, fixture('server-assistant-end.json')).state;
    expect(isWorking(ended)).toBe(false);

    const queued = { ...idle, queue: 1 };
    expect(isWorking(queued)).toBe(true);
  });

  it('does not latch on when a stream stops without its end', () => {
    let state = applyEvent(emptySession(), fixture('server-assistant-start.json')).state;
    expect(isWorking(state)).toBe(true);
    // A stop or a failed stream ends the turn with a notice and no assistant_end.
    state = applyEvent(state, fixture('server-notice.json')).state;
    expect(isWorking(state)).toBe(false);

    let failed = applyEvent(emptySession(), fixture('server-assistant-start.json')).state;
    failed = applyEvent(failed, fixture('server-error.json')).state;
    expect(isWorking(failed)).toBe(false);
  });

  it('is false once the session has closed, whatever is left on screen', () => {
    const saved = applyEvent(emptySession(), fixture('server-saved.json')).state;
    expect(isWorking({ ...saved, queue: 1 })).toBe(false);
  });
});

describe('the one transcript order', () => {
  it('interleaves a message, its activity and the card that follows', () => {
    let state = emptySession();
    state = applyEvent(state, fixture('server-user-message.json')).state;
    state = applyEvent(state, fixture('server-tool-start.json')).state;
    state = applyEvent(state, fixture('server-tool-end.json')).state;
    state = applyEvent(state, fixture('server-interaction-choose-medium.json')).state;

    const rows = timeline(state);
    expect(rows.map((row) => row.kind)).toEqual(['block', 'activity', 'interaction']);
    // The end of a tool lands on the row its start made, not on a second row after it.
    expect(rows.filter((row) => row.kind === 'activity')).toHaveLength(1);
    expect(rows[1].kind === 'activity' && rows[1].row.live).toBe(false);
  });

  it('settles a tool left running when the turn ends on a stop, a timeout or an error', () => {
    // The composer reads a live activity row as a reply in progress, so a row that never gets its end would keep
    // it on Stop for a turn that is over.
    const running = applyEvent(emptySession(), fixture('server-tool-start.json')).state;
    expect(isWorking(running)).toBe(true);

    const notice = fixture('server-notice.json');
    const stopped = {
      ...notice,
      data: { ...(notice.data as object), code: 'stopped' },
    } as unknown as ServerEvent;
    expect(isWorking(applyEvent(running, stopped).state)).toBe(false);

    expect(isWorking(applyEvent(running, fixture('server-error.json')).state)).toBe(false);

    // A notice in the middle of a turn leaves the running tool alone, so its own end still finds it.
    const mid = {
      ...notice,
      data: { ...(notice.data as object), code: 'drops_opinion' },
    } as unknown as ServerEvent;
    expect(isWorking(applyEvent(running, mid).state)).toBe(true);
  });

  it('keeps a notice where it arrived, between the message and the reply', () => {
    let state = emptySession();
    state = applyEvent(state, fixture('server-user-message.json')).state;
    state = applyEvent(state, fixture('server-notice.json')).state;
    state = applyEvent(state, fixture('server-assistant-start.json')).state;
    const rows = timeline(state);
    expect(rows.map((row) => row.kind)).toEqual(['block', 'notice', 'block']);
    expect(rows[1].kind === 'notice' && rows[1].row.code).toBe('provider_down');
  });

  it('numbers every row from one counter, so a replay rebuilds the same screen', () => {
    const events = [
      fixture('server-user-message.json'),
      fixture('server-activity.json'),
      fixture('server-interaction-choose-medium.json'),
    ];
    const once = applyAll(emptySession(), events);
    const twice = applyAll(emptySession(), events);
    expect(timeline(once)).toEqual(timeline(twice));
    expect(timeline(once).map((row) => row.order)).toEqual([1, 2, 3]);
  });
});

describe('her answers on their way to the card', () => {
  function at(seq: number, type: string, data: Record<string, unknown>): ServerEvent {
    return {
      seq,
      type,
      session_id: 's_8f2',
      ts: '2026-11-23T12:00:00Z',
      data,
    } as unknown as ServerEvent;
  }

  function request(kind: string): SessionState {
    return applyEvent(
      emptySession('s_8f2'),
      at(1, 'interaction_request', { interaction_id: 'i1', kind, payload: {} }),
    ).state;
  }

  it('keeps Save waiting from her tap until a card written after it', () => {
    // The agent stores her answers when it writes the card, so the gap between the tap and that write is exactly when a
    // save would store the card built before she answered (issue #46).
    let state = request('describe');
    expect(state.answersPending).toBe(false);

    state = applyEvent(
      state,
      at(2, 'interaction_resolved', { by: 'user', interaction_id: 'i1', answers_pending: true }),
    ).state;
    expect(state.interactions).toHaveLength(0);
    expect(state.answersPending).toBe(true);

    // A card written before her answer is stored still says the wait is on; the one published after it clears it.
    state = applyEvent(
      state,
      at(3, 'draft_snapshot', { version: 5, draft: { version: 5, answers_pending: true } }),
    ).state;
    expect(state.answersPending).toBe(true);

    state = applyEvent(
      state,
      at(4, 'draft_snapshot', { version: 5, draft: { version: 5, answers_pending: false } }),
    ).state;
    expect(state.answersPending).toBe(false);
  });

  it('does not wait when the resolution carried no answer of hers', () => {
    // A skip, or a lookup choice, carries nothing for the card: nothing is pending and Save is not held up.
    for (const kind of ['describe', 'choose_candidate']) {
      let state = request(kind);
      state = applyEvent(
        state,
        at(2, 'interaction_resolved', { by: 'user', interaction_id: 'i1' }),
      ).state;
      expect(state.answersPending, kind).toBe(false);
    }
  });

  it('takes the wait from the card a hello carries', () => {
    const hello = {
      seq: null,
      type: 'hello',
      session_id: 's_8f2',
      ts: '2026-11-23T12:00:00Z',
      data: {
        session: { id: 's_8f2', kind: 'entry', state: 'active' },
        state: 'active',
        draft: { version: 3, answers_pending: true },
        open_interactions: [],
        head_seq: 9,
        full_state: true,
        server_version: 'dev',
      },
    } as unknown as ServerEvent;
    expect(applyHello(emptySession('s_8f2'), hello).answersPending).toBe(true);
    expect(
      applyHello(emptySession('s_8f2'), {
        ...hello,
        data: { ...hello.data, draft: null },
      } as unknown as ServerEvent).answersPending,
    ).toBe(false);
  });
});
