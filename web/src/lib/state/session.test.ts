import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { checkServerEvent } from '../contract/validate';
import type { ServerEvent } from '../contract/events';
import { applyAll, applyEvent, applyHello, emptySession } from './session';
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
