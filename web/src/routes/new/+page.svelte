<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createClient, ApiError } from '../../lib/api/client';

  /**
   * Start an entry session and go straight to it. Edit on a saved card comes through here with `?reopen=<entry>`,
   * so the new session opens a revision of that entry as soon as the socket says hello.
   */
  const client = createClient();
  const reopen = page.url.searchParams.get('reopen') ?? '';
  let problem = $state('');

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const session = await client.createSession({ kind: 'entry' });
        if (cancelled) return;
        const query = reopen ? `?reopen=${encodeURIComponent(reopen)}` : '';
        // resolve() builds the path and the base; the one-shot query is added to it.
        // eslint-disable-next-line svelte/no-navigation-without-resolve
        await goto(resolve('/s/[id]', { id: session.id }) + query, { replaceState: true });
      } catch (error) {
        if (cancelled) return;
        problem = error instanceof ApiError ? error.copy : 'Sepia could not start a session.';
      }
    })();
    return () => {
      cancelled = true;
    };
  });
</script>

<svelte:head>
  <title>New entry · Sepia</title>
</svelte:head>

<main>
  <p>Starting a new entry.</p>
  {#if problem}
    <p class="problem" role="alert">{problem}</p>
    <a href={resolve('/')}>Back to the library</a>
  {/if}
</main>

<style>
  main {
    min-height: 100dvh;
    display: grid;
    place-content: center;
    gap: 0.75rem;
    padding: 2rem;
    text-align: center;
  }
  p {
    margin: 0;
    color: var(--text-2);
  }
  .problem {
    color: var(--danger);
  }
  a {
    color: var(--text-2);
  }
</style>
