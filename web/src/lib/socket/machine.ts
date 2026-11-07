import type { ClientMessage, ServerEvent } from '../contract/events';

/**
 * The session socket as a state machine, so every transition can be tested in Vitest without a browser
 * (docs/STANDARDS.md 5). It owns the connection, the resume point and the offline outbox; it owns no UI and no
 * store. The page wires it to a store and a DOM socket; a test wires it to a fake.
 *
 * The rules it implements are docs/PROTOCOL.md 1 and 6: hello, replay from `last_seq`, a 45 second silence is a
 * dead socket, backoff of 0.5 to 15 seconds with jitter, an immediate attempt when the page wakes, and a message
 * that is typed while the socket is down is kept and sent with its original `client_msg_id`.
 */

export type ConnectionState = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'closed';

/** The bit of a WebSocket the machine uses. A fake in a test is three methods and no browser. */
export interface SocketLike {
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export interface SocketHandlers {
  onMessage(raw: string): void;
  onClose(): void;
}

export type Connect = (url: string, handlers: SocketHandlers) => SocketLike;

export interface AckData {
  client_msg_id?: string;
  status?: 'accepted' | 'duplicate' | 'rejected' | 'queued';
  reason?: string;
}

export interface MachineOptions {
  /** The path to the socket, without a query. The machine adds `last_seq` itself. */
  url: string;
  connect?: Connect;
  random?: () => number;
  newID?: () => string;
  onStateChange?: (state: ConnectionState) => void;
  onHello?: (hello: ServerEvent) => void;
  onEvent?: (event: ServerEvent) => void;
  onAck?: (ack: AckData) => void;
  /** The service was updated under an open page: the honest response is a reload. */
  onReload?: () => void;
  /** A frame that is not the contract, or an event the client cannot use. Never fatal. */
  onIgnored?: (reason: string, detail: unknown) => void;
}

const BASE_DELAY = 500;
const MAX_DELAY = 15_000;
const JITTER = 0.2;
/** The server pings every 20 seconds, so a socket silent for 45 is dead (docs/PROTOCOL.md 1). */
export const SILENCE_LIMIT = 45_000;
const RELOAD_DELAY = 1_500;

function defaultConnect(url: string, handlers: SocketHandlers): SocketLike {
  const ws = new WebSocket(url);
  ws.addEventListener('message', (event) => {
    if (typeof event.data === 'string') {
      handlers.onMessage(event.data);
    } else {
      // The protocol is text. A binary frame is a server bug, and dropping it is kinder than throwing.
      handlers.onMessage('');
    }
  });
  ws.addEventListener('close', () => handlers.onClose());
  return {
    send: (data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data);
    },
    close: (code, reason) => ws.close(code, reason),
  };
}

/**
 * The smallest shape the machine insists on. Anything else is ignored, logged and left out of the log: a
 * malformed frame must not be able to stop the app (S10 bad route).
 */
function asEvent(value: unknown): ServerEvent | null {
  if (typeof value !== 'object' || value === null) return null;
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.type !== 'string' || typeof candidate.session_id !== 'string') return null;
  if (candidate.seq !== null && typeof candidate.seq !== 'number') return null;
  return candidate as unknown as ServerEvent;
}

export class SessionSocket {
  private readonly options: MachineOptions;
  private readonly connect: Connect;
  private readonly random: () => number;
  private readonly newID: () => string;

  private socket: SocketLike | null = null;
  private state: ConnectionState = 'idle';
  private lastSeq = 0;
  private attempts = 0;
  private stopped = false;
  private serverVersion: string | null = null;
  private outbox = new Map<string, ClientMessage>();
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private silenceTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(options: MachineOptions) {
    this.options = options;
    this.connect = options.connect ?? defaultConnect;
    this.random = options.random ?? Math.random;
    this.newID = options.newID ?? (() => crypto.randomUUID());
  }

  get connectionState(): ConnectionState {
    return this.state;
  }

  /** The highest persisted sequence the page has seen. A reconnection resumes from here. */
  get resumePoint(): number {
    return this.lastSeq;
  }

  /** How many messages are waiting for an ack, including one typed offline. */
  get pending(): number {
    return this.outbox.size;
  }

  /** Open the socket. Calling it twice is harmless: the second call sees a live connection. */
  start(): void {
    if (this.socket !== null) return;
    this.stopped = false;
    this.open();
  }

  private setState(state: ConnectionState): void {
    if (this.state === state) return;
    this.state = state;
    this.options.onStateChange?.(state);
  }

  private url(): string {
    if (this.lastSeq === 0) return this.options.url;
    const separator = this.options.url.includes('?') ? '&' : '?';
    return `${this.options.url}${separator}last_seq=${this.lastSeq}`;
  }

  private open(): void {
    if (this.stopped) return;
    this.clearRetry();
    this.setState(this.attempts === 0 ? 'connecting' : 'reconnecting');
    try {
      this.socket = this.connect(this.url(), {
        onMessage: (raw) => this.onMessage(raw),
        onClose: () => this.onClose(),
      });
    } catch (error) {
      // A constructor that throws (a bad URL, a browser without WebSockets) is a drop like any other.
      this.options.onIgnored?.('the socket could not be opened', error);
      this.scheduleRetry();
      return;
    }
    this.armSilenceWatchdog();
  }

  private onMessage(raw: string): void {
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch {
      this.options.onIgnored?.('a frame that is not JSON', raw);
      return;
    }
    const event = asEvent(parsed);
    if (event === null) {
      this.options.onIgnored?.('a frame that is not an event envelope', parsed);
      return;
    }
    this.armSilenceWatchdog();

    if (event.type === 'hello') {
      this.onHello(event);
      return;
    }
    if (typeof event.seq === 'number') {
      // A replay that overlaps what we already have is not an error; it is how a resume is allowed to be
      // generous. Drop it rather than showing the same sentence twice (docs/PROTOCOL.md 6).
      if (event.seq <= this.lastSeq) return;
      this.lastSeq = event.seq;
    }
    if (event.type === 'ack') {
      this.handleAck(event);
    }
    this.options.onEvent?.(event);
  }

  private onHello(event: ServerEvent): void {
    const data = (event.data ?? {}) as {
      server_version?: unknown;
      full_state?: unknown;
      head_seq?: unknown;
    };
    const version = typeof data.server_version === 'string' ? data.server_version : null;
    if (this.serverVersion !== null && version !== null && version !== this.serverVersion) {
      // The service was updated while this page was open. A reload is the only honest response, and it is
      // delayed a moment so the current frame is not torn away mid-paint. No reconnect runs in the meantime.
      this.stopped = true;
      this.closeSocket(1000, 'reloading');
      this.setState('closed');
      setTimeout(() => this.options.onReload?.(), RELOAD_DELAY);
      return;
    }
    this.serverVersion = version;
    if (data.full_state === true && typeof data.head_seq === 'number') {
      // The log was compacted past our point. The snapshot in this hello is the state, so the next resume
      // starts at the head rather than asking for events that no longer exist.
      this.lastSeq = data.head_seq;
    }
    this.attempts = 0;
    this.setState('open');
    this.options.onHello?.(event);
    this.flush();
  }

  private handleAck(event: ServerEvent): void {
    const data = (event.data ?? {}) as AckData;
    const id = data.client_msg_id;
    if (typeof id === 'string') {
      this.outbox.delete(id);
    }
    this.options.onAck?.(data);
  }

  /**
   * Send a client message. While the socket is down the message waits in the outbox, and it stays there until an
   * ack arrives, so a reconnect re-sends it and the server's `client_msg_id` memory turns the retry into a
   * duplicate rather than a second effect.
   */
  send(message: ClientMessage): string {
    const withID = message.client_msg_id ? message : { ...message, client_msg_id: this.newID() };
    const id = withID.client_msg_id as string;
    if (this.outbox.has(id)) return id;
    this.outbox.set(id, withID);
    if (this.state === 'open') this.deliver(withID);
    return id;
  }

  /** Drop a message that has not been sent yet, for the "never mind" a person can always choose. */
  cancel(clientMsgID: string): void {
    this.outbox.delete(clientMsgID);
  }

  private flush(): void {
    for (const message of this.outbox.values()) this.deliver(message);
  }

  private deliver(message: ClientMessage): void {
    if (this.socket === null) return;
    try {
      this.socket.send(JSON.stringify(message));
    } catch (error) {
      // A send on a socket that has just died is not a lost message: it is still in the outbox and goes again
      // on the next connection.
      this.options.onIgnored?.('a send failed; the message is still queued', error);
    }
  }

  private onClose(): void {
    this.socket = null;
    this.clearSilenceWatchdog();
    if (this.stopped) {
      this.setState('closed');
      return;
    }
    this.scheduleRetry();
  }

  /** The page became visible, or the network came back: try now rather than waiting out the backoff. */
  wake(): void {
    if (this.stopped || this.state === 'open' || this.state === 'connecting') return;
    this.clearRetry();
    this.open();
  }

  stop(): void {
    this.stopped = true;
    this.clearRetry();
    this.clearSilenceWatchdog();
    this.closeSocket(1000, 'the client is closing');
    this.setState('closed');
  }

  private closeSocket(code: number, reason: string): void {
    const socket = this.socket;
    this.socket = null;
    this.clearSilenceWatchdog();
    try {
      socket?.close(code, reason);
    } catch {
      // Closing a socket that is already gone is not an error worth reporting.
    }
  }

  private scheduleRetry(): void {
    if (this.stopped || this.retryTimer !== null) return;
    this.setState('reconnecting');
    const delay = this.backoff();
    this.attempts += 1;
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      this.open();
    }, delay);
    // A retry timer needs unref only in Node; wrapping it keeps the browser behaviour identical.
    (this.retryTimer as { unref?: () => void }).unref?.();
  }

  private backoff(): number {
    const base = Math.min(BASE_DELAY * 2 ** this.attempts, MAX_DELAY);
    // ±20% jitter, so a phone coming out of a tunnel does not have every tab retry on the same tick.
    const factor = 1 + (this.random() * 2 - 1) * JITTER;
    return Math.round(base * factor);
  }

  private clearRetry(): void {
    if (this.retryTimer !== null) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  private armSilenceWatchdog(): void {
    this.clearSilenceWatchdog();
    this.silenceTimer = setTimeout(() => {
      this.silenceTimer = null;
      // Silence is a dead socket even if the browser has not noticed. Close it so the retry path runs with
      // the same rules as a real drop.
      this.closeSocket(4000, 'nothing heard in 45 seconds');
      this.onClose();
    }, SILENCE_LIMIT);
    (this.silenceTimer as { unref?: () => void }).unref?.();
  }

  private clearSilenceWatchdog(): void {
    if (this.silenceTimer !== null) {
      clearTimeout(this.silenceTimer);
      this.silenceTimer = null;
    }
  }
}
