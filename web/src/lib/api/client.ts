import type { components, paths } from '../contract/api';
import { errorCodes, type ErrorCode } from '../contract/codes';

/**
 * The one place the app talks to the server over HTTP. A component never calls `fetch`: it calls a method here,
 * which returns a typed body or throws an `ApiError` carrying the contract's stable code and the copy the UI
 * shows. Types come from the generated contract (docs/STANDARDS.md 5).
 */

type Operation<P extends keyof paths, M extends keyof paths[P]> = paths[P][M];

/** The success body of an operation: a 200 or a 201 with a JSON content type. */
type Ok<T> = T extends { responses: infer R }
  ? R extends { 200: { content: { 'application/json': infer B } } }
    ? B
    : R extends { 201: { content: { 'application/json': infer B } } }
      ? B
      : never
  : never;

export type Session = components['schemas']['Session'];
export type Entry = components['schemas']['Entry'];
export type SessionPage = Ok<Operation<'/api/sessions', 'get'>>;
export type LibraryPage = Ok<Operation<'/api/entries', 'get'>>;
export type RevisionPage = Ok<Operation<'/api/entries/{id}/revisions', 'get'>>;
export type WorkDetail = Ok<Operation<'/api/works/{id}', 'get'>>;
export type SearchResults = Ok<Operation<'/api/search', 'get'>>;
export type VersionInfo = Ok<Operation<'/api/version', 'get'>>;
export type ServicesInfo = Ok<Operation<'/api/services', 'get'>>;

/** The error body every failing route answers with. */
interface ErrorEnvelope {
  code: string;
  message: string;
}

/** A refusal from the server, carrying the code the contract names so the UI can say something true. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }

  /** The plain sentence for this code, from the generated vocabulary. Falls back to what the server sent. */
  get copy(): string {
    const known = errorCodes[this.code as ErrorCode];
    return known ? known.copy : this.message;
  }
}

export interface ClientOptions {
  /** Empty means the page's own origin, which is what the app uses behind the tailnet name. */
  baseUrl?: string;
  fetch?: typeof fetch;
  /** A request that has not answered in this long is abandoned. */
  timeoutMs?: number;
}

export interface SessionQuery {
  state?: components['schemas']['SessionState'];
  kind?: components['schemas']['SessionKind'];
  cursor?: string;
  limit?: number;
}

export interface LibraryQuery {
  medium?: components['schemas']['Medium'];
  cursor?: string;
  limit?: number;
  q?: string;
}

export interface Client {
  createSession(body: components['schemas']['CreateSession']): Promise<Session>;
  getSession(id: string): Promise<Session>;
  listSessions(query?: SessionQuery): Promise<SessionPage>;
  listEntries(query?: LibraryQuery): Promise<LibraryPage>;
  getEntry(id: string): Promise<Entry>;
  listRevisions(id: string, cursor?: string): Promise<RevisionPage>;
  deleteEntry(id: string, baseVersion: number): Promise<void>;
  getWork(id: string): Promise<WorkDetail>;
  search(q: string): Promise<SearchResults>;
  getVersion(): Promise<VersionInfo>;
  getServices(): Promise<ServicesInfo>;
}

function queryString(query: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '') params.set(key, String(value));
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : '';
}

export function createClient(options: ClientOptions = {}): Client {
  const baseUrl = (options.baseUrl ?? '').replace(/\/$/, '');
  const doFetch = options.fetch ?? globalThis.fetch;
  const timeoutMs = options.timeoutMs ?? 20_000;

  async function request<T>(method: 'GET' | 'POST', path: string, body?: unknown): Promise<T> {
    const signal = AbortSignal.timeout(timeoutMs);
    const response = await doFetch(baseUrl + path, {
      method,
      credentials: 'same-origin',
      headers:
        body === undefined
          ? { Accept: 'application/json' }
          : {
              Accept: 'application/json',
              'Content-Type': 'application/json',
            },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    });
    if (response.status === 204) return undefined as T;
    if (!response.ok) {
      // A refusal is the contract's envelope. Anything else (a proxy's HTML 502, a truncated body) is not the
      // app's, and must still arrive as one error type rather than a parse crash.
      const envelope: ErrorEnvelope = {
        code: 'internal',
        message: 'Something went wrong. Try again.',
      };
      try {
        const body = (await response.json()) as Partial<ErrorEnvelope>;
        if (typeof body.code === 'string') envelope.code = body.code;
        if (typeof body.message === 'string') envelope.message = body.message;
      } catch {
        // keep the default
      }
      throw new ApiError(response.status, envelope.code, envelope.message);
    }
    try {
      return (await response.json()) as T;
    } catch {
      throw new ApiError(response.status, 'internal', 'The server answered something unreadable.');
    }
  }

  return {
    createSession: (body) => request('POST', '/api/sessions', body),
    getSession: (id) => request('GET', `/api/sessions/${encodeURIComponent(id)}`),
    listSessions: (query = {}) => request('GET', `/api/sessions${queryString({ ...query })}`),
    listEntries: (query = {}) => request('GET', `/api/entries${queryString({ ...query })}`),
    getEntry: (id) => request('GET', `/api/entries/${encodeURIComponent(id)}`),
    listRevisions: (id, cursor) =>
      request('GET', `/api/entries/${encodeURIComponent(id)}/revisions${queryString({ cursor })}`),
    deleteEntry: async (id, baseVersion) => {
      await request<unknown>('POST', `/api/entries/${encodeURIComponent(id)}/delete`, {
        base_version: baseVersion,
      });
    },
    getWork: (id) => request('GET', `/api/works/${encodeURIComponent(id)}`),
    search: (q) => request('GET', `/api/search${queryString({ q })}`),
    getVersion: () => request('GET', '/api/version'),
    getServices: () => request('GET', '/api/services'),
  };
}
