import { describe, expect, it } from 'vitest';
import { ApiError, createClient } from './client';

/**
 * The typed client. A fake `fetch` records the request and returns a canned response, so the three routes are:
 * the shape a route answers with (happy), a refusal envelope (bad), and a body that is not the contract at all,
 * which is what a proxy or a half-restarted service produces (sad).
 */

interface Call {
  url: string;
  init: RequestInit;
}

function fakeFetch(status: number, body: unknown, contentType = 'application/json') {
  const calls: Call[] = [];
  const fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    calls.push({ url: String(input), init });
    const text = typeof body === 'string' ? body : JSON.stringify(body);
    return new Response(status === 204 ? null : text, {
      status,
      headers: body === undefined ? {} : { 'Content-Type': contentType },
    });
  }) as typeof globalThis.fetch;
  return { calls, fetch };
}

describe('the API client', () => {
  it('creates a session with a JSON body and same-origin credentials', async () => {
    const { calls, fetch } = fakeFetch(201, { id: 's1', kind: 'entry', state: 'active' });
    const client = createClient({ fetch });

    const session = await client.createSession({ kind: 'entry' });

    expect(session.id).toBe('s1');
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe('/api/sessions');
    expect(calls[0].init.method).toBe('POST');
    expect(calls[0].init.credentials).toBe('same-origin');
    expect(calls[0].init.body).toBe('{"kind":"entry"}');
    expect((calls[0].init.headers as Record<string, string>)['Content-Type']).toBe(
      'application/json',
    );
  });

  it('builds a query string from the parts that are set and leaves the rest out', async () => {
    const { calls, fetch } = fakeFetch(200, { items: [], next_cursor: null });
    const client = createClient({ fetch, baseUrl: 'https://example.test/' });

    await client.listSessions({ state: 'active', limit: 20, cursor: undefined, kind: '' as never });

    expect(calls[0].url).toBe('https://example.test/api/sessions?state=active&limit=20');
  });

  it('encodes an id rather than pasting it into the path', async () => {
    const { calls, fetch } = fakeFetch(200, {});
    const client = createClient({ fetch });
    await client.getEntry('a b/c');
    expect(calls[0].url).toBe('/api/entries/a%20b%2Fc');
  });

  it('turns the refusal envelope into an ApiError with the code and its plain copy', async () => {
    const { fetch } = fakeFetch(409, {
      code: 'session_closed',
      message: 'This conversation is finished.',
    });
    const client = createClient({ fetch });

    const error = await client.getSession('s1').catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.status).toBe(409);
    expect(apiError.code).toBe('session_closed');
    // The copy comes from the generated vocabulary, not from whatever prose the server happened to send.
    expect(apiError.copy).toBe('This conversation is finished. Start a new one.');
  });

  it('still fails as one error type when the body is not the contract', async () => {
    const { fetch } = fakeFetch(502, '<html>bad gateway</html>', 'text/html');
    const client = createClient({ fetch });

    const error = await client.listEntries().catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe('internal');
  });

  it('fails as an ApiError when a success body cannot be read', async () => {
    const { fetch } = fakeFetch(200, 'not json at all');
    const client = createClient({ fetch });

    const error = await client.getWork('w1').catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe('internal');
  });

  it('sends the base version with a delete and accepts an empty reply', async () => {
    const { calls, fetch } = fakeFetch(204, undefined);
    const client = createClient({ fetch });

    await expect(client.deleteEntry('e1', 3)).resolves.toBeUndefined();
    expect(calls[0].init.body).toBe('{"base_version":3}');
    expect(calls[0].url).toBe('/api/entries/e1/delete');
  });
});
