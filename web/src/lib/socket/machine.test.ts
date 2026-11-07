import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ClientMessage, ServerEvent } from '../contract/events';
import {
  SessionSocket,
  SILENCE_LIMIT,
  type ConnectionState,
  type SocketHandlers,
  type SocketLike,
} from './machine';

/**
 * The socket machine, driven by a fake socket and fake timers. The three routes from the slice plan are marked:
 * connect and resume in order (happy), a frame or event the client cannot use (bad), and a drop, a stall or a
 * version change under an open page (sad).
 */

class FakeSocket implements SocketLike {
  readonly url: string;
  sent: string[] = [];
  closed: { code?: number; reason?: string } | null = null;
  handlers: SocketHandlers;

  constructor(url: string, handlers: SocketHandlers) {
    this.url = url;
    this.handlers = handlers;
  }

  send(data: string): void {
    if (this.closed) throw new Error('send on a closed socket');
    this.sent.push(data);
  }

  close(code?: number, reason?: string): void {
    this.closed = { code, reason };
    // A real socket reports its own close; the fake does too, so the machine cannot pass a test by relying on
    // an event the browser would have sent and the fake never did.
    queueMicrotask(() => this.handlers.onClose());
  }

  /** The server sends a frame. */
  deliver(value: unknown): void {
    this.handlers.onMessage(typeof value === 'string' ? value : JSON.stringify(value));
  }
}

function event(type: string, seq: number | null, data: Record<string, unknown> = {}): ServerEvent {
  return {
    seq,
    type,
    session_id: 's1',
    ts: '2026-11-08T12:00:00Z',
    data,
  } as unknown as ServerEvent;
}

function hello(overrides: Record<string, unknown> = {}): ServerEvent {
  return event('hello', null, {
    session: { id: 's1', kind: 'entry', state: 'active' },
    state: 'active',
    draft: null,
    open_interactions: [],
    head_seq: 0,
    full_state: false,
    server_version: 'test',
    ...overrides,
  });
}

function rig(options: { random?: () => number } = {}) {
  const sockets: FakeSocket[] = [];
  const states: ConnectionState[] = [];
  const events: ServerEvent[] = [];
  const hellos: ServerEvent[] = [];
  const acks: { client_msg_id?: string; status?: string; reason?: string }[] = [];
  const ignored: string[] = [];
  const reloads: string[] = [];
  let ids = 0;

  const machine = new SessionSocket({
    url: '/ws/sessions/s1',
    random: options.random ?? (() => 0.5),
    newID: () => `m-${++ids}`,
    connect: (url, handlers) => {
      const socket = new FakeSocket(url, handlers);
      sockets.push(socket);
      return socket;
    },
    onStateChange: (state) => states.push(state),
    onHello: (value) => hellos.push(value),
    onEvent: (value) => events.push(value),
    onAck: (ack) => acks.push(ack),
    onIgnored: (reason) => ignored.push(reason),
    onReload: () => reloads.push('reload'),
  });

  const last = () => sockets[sockets.length - 1];
  return { machine, sockets, states, events, hellos, acks, ignored, reloads, last };
}

const text: ClientMessage = {
  type: 'message',
  client_msg_id: 'typed-1',
  data: { text: 'finished dune last night', client_date: '2026-11-08', tz: 'Europe/London' },
} as unknown as ClientMessage;

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
});

describe('the socket machine', () => {
  it('connects, opens on hello and delivers events in order', () => {
    const r = rig();
    r.machine.start();
    expect(r.states).toEqual(['connecting']);
    expect(r.last().url).toBe('/ws/sessions/s1');

    r.last().deliver(hello({ head_seq: 2 }));
    expect(r.states).toEqual(['connecting', 'open']);
    expect(r.hellos).toHaveLength(1);

    r.last().deliver(event('user_message', 1, { text: 'a' }));
    r.last().deliver(event('user_message', 2, { text: 'b' }));
    expect(r.events.map((e) => e.seq)).toEqual([1, 2]);
    expect(r.machine.resumePoint).toBe(2);
  });

  it('resumes from last_seq after a drop, and backs off 0.5, 1 and 2 seconds', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.last().deliver(event('user_message', 7, { text: 'seen' }));
    expect(r.machine.resumePoint).toBe(7);

    r.last().handlers.onClose();
    expect(r.states.at(-1)).toBe('reconnecting');
    expect(r.sockets).toHaveLength(1);
    vi.advanceTimersByTime(499);
    expect(r.sockets).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(r.sockets).toHaveLength(2);
    expect(r.last().url).toBe('/ws/sessions/s1?last_seq=7');

    // Each drop doubles the wait: 1 s, then 2 s. A successful hello resets it afterwards.
    r.last().handlers.onClose();
    vi.advanceTimersByTime(1000);
    expect(r.sockets).toHaveLength(3);
    r.last().handlers.onClose();
    vi.advanceTimersByTime(2000);
    expect(r.sockets).toHaveLength(4);
    r.last().deliver(hello());
    r.last().handlers.onClose();
    vi.advanceTimersByTime(500);
    expect(r.sockets).toHaveLength(5);
  });

  it('caps the backoff at 15 seconds', () => {
    const r = rig({ random: () => 0.5 });
    r.machine.start();
    for (let i = 0; i < 10; i += 1) {
      r.last().handlers.onClose();
      vi.advanceTimersByTime(15_000);
    }
    expect(r.sockets.length).toBe(11);
  });

  it('keeps a message typed offline and sends it once, in order, with its original id', () => {
    const r = rig();
    r.machine.start();
    const first = r.machine.send(text);
    const second = r.machine.send({
      ...text,
      client_msg_id: 'typed-2',
      data: { text: 'and the sequel', client_date: '2026-11-08', tz: 'Europe/London' },
    } as unknown as ClientMessage);

    // Nothing goes out before hello, because the server might not be ready to apply it.
    expect(r.last().sent).toEqual([]);
    expect(first).toBe('typed-1');
    expect(second).toBe('typed-2');
    expect(r.machine.pending).toBe(2);

    r.last().deliver(hello());
    expect(r.last().sent.map((raw) => JSON.parse(raw).client_msg_id)).toEqual([
      'typed-1',
      'typed-2',
    ]);
    expect(r.machine.pending).toBe(2);

    r.last().deliver(event('ack', 1, { client_msg_id: 'typed-1', status: 'accepted' }));
    expect(r.machine.pending).toBe(1);
  });

  it('re-sends an unacked message after a drop, and the server sees the same id', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.machine.send(text);
    expect(r.last().sent).toHaveLength(1);

    r.last().handlers.onClose();
    vi.advanceTimersByTime(500);
    r.last().deliver(hello());
    expect(r.last().sent).toHaveLength(1);
    expect(JSON.parse(r.last().sent[0]).client_msg_id).toBe('typed-1');
  });

  it('drops a replayed event rather than showing it twice', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.last().deliver(event('user_message', 3, { text: 'once' }));
    r.last().handlers.onClose();
    vi.advanceTimersByTime(500);
    r.last().deliver(hello());
    // A generous replay: the server repeats seq 3 as well as what follows.
    r.last().deliver(event('user_message', 3, { text: 'once' }));
    r.last().deliver(event('user_message', 4, { text: 'twice' }));
    expect(r.events.map((e) => e.seq)).toEqual([3, 4]);
  });

  it('ignores a malformed or unknown frame without closing', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.last().deliver('not json at all');
    r.last().deliver({ type: 42 });
    r.last().deliver({ seq: 1 });
    expect(r.ignored.length).toBe(3);
    expect(r.states.at(-1)).toBe('open');
    // An event type this client has never heard of is still handed over; the store decides what to do with it.
    r.last().deliver(event('something_new', 5, {}));
    expect(r.events.map((e) => e.type)).toEqual(['something_new']);
    expect(r.machine.resumePoint).toBe(5);
  });

  it('reloads once when the server version changes under an open page', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello({ server_version: 'one' }));
    r.last().handlers.onClose();
    vi.advanceTimersByTime(500);
    r.last().deliver(hello({ server_version: 'two' }));
    expect(r.states.at(-1)).toBe('closed');
    expect(r.reloads).toEqual([]);
    vi.advanceTimersByTime(1_500);
    expect(r.reloads).toEqual(['reload']);
    // Even after the close, no retry runs: the page is on its way to a reload.
    vi.advanceTimersByTime(30_000);
    expect(r.sockets).toHaveLength(2);
  });

  it('treats 45 seconds of silence as a dead socket', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    vi.advanceTimersByTime(SILENCE_LIMIT - 1);
    expect(r.states.at(-1)).toBe('open');
    vi.advanceTimersByTime(1);
    expect(r.states.at(-1)).toBe('reconnecting');
    vi.advanceTimersByTime(500);
    expect(r.sockets).toHaveLength(2);
  });

  it('keeps the watchdog fed by frames, so an active socket is never cut', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    for (let i = 0; i < 5; i += 1) {
      vi.advanceTimersByTime(40_000);
      r.last().deliver(event('cost_update', null, {}));
    }
    expect(r.sockets).toHaveLength(1);
    expect(r.states.at(-1)).toBe('open');
  });

  it('retries at once when the page wakes instead of waiting out the backoff', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.last().handlers.onClose();
    r.machine.wake();
    expect(r.sockets).toHaveLength(2);
    r.last().deliver(hello());
    // The abandoned retry timer must not open a third socket later. 40 s is under the silence limit, so a
    // third socket here could only be that stale timer.
    vi.advanceTimersByTime(40_000);
    expect(r.sockets).toHaveLength(2);
  });

  it('does not reconnect after a deliberate stop', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello());
    r.machine.stop();
    expect(r.states.at(-1)).toBe('closed');
    expect(r.last().closed?.code).toBe(1000);
    vi.advanceTimersByTime(60_000);
    expect(r.sockets).toHaveLength(1);
  });

  it('jumps the resume point to head_seq when the log was compacted', () => {
    const r = rig();
    r.machine.start();
    r.last().deliver(hello({ full_state: true, head_seq: 42 }));
    expect(r.machine.resumePoint).toBe(42);
    r.last().handlers.onClose();
    vi.advanceTimersByTime(500);
    expect(r.last().url).toBe('/ws/sessions/s1?last_seq=42');
  });
});
