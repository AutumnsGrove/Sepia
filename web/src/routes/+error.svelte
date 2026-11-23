<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { RotateCcw } from '@lucide/svelte';

  /**
   * One screen's worth of failure, caught.
   *
   * A render error used to replace the whole route with SvelteKit's bare "500 Internal Error": no title, no back,
   * nothing to tap, and the only way out was to leave the app. A stored entry with a repeated "why" check did
   * exactly that (issue #38). What should fail is the screen, not the app.
   *
   * SvelteKit renders this page inside the layout, for a failed `load` and for a component that throws while it
   * draws. The copy is one sentence and two ways forward, and it never shows the error's own text: that can carry a
   * row or a path she does not need to see.
   */
  const missing = $derived(page.status === 404);

  function back() {
    if (window.history.length > 1) window.history.back();
    else void goto(resolve('/'));
  }

  function reload() {
    window.location.reload();
  }
</script>

<svelte:head>
  <title>{missing ? 'Not found' : 'This screen did not load'} · Sepia</title>
</svelte:head>

<main class="screen">
  <div class="box" role="alert">
    <h1>{missing ? 'There is nothing here' : 'This screen did not load'}</h1>
    <p>
      {missing
        ? 'That page does not exist. Go back, or start from home.'
        : 'Something went wrong drawing it. Your reviews are safe. Go back, or reload the page.'}
    </p>
    <div class="acts">
      <button type="button" class="back" onclick={back}>Back</button>
      {#if missing}
        <a class="home" href={resolve('/')}>Home</a>
      {:else}
        <button type="button" class="reload" onclick={reload}>
          <RotateCcw size={16} aria-hidden="true" />
          Reload
        </button>
      {/if}
    </div>
  </div>
</main>

<style>
  .screen {
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    padding-top: max(1.5rem, env(safe-area-inset-top));
    padding-bottom: max(1.5rem, env(safe-area-inset-bottom));
  }
  .box {
    max-width: 32rem;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    padding: 1.5rem;
  }
  h1 {
    margin: 0 0 0.5rem;
    font:
      500 1.375rem/1.2 'Fraunces',
      serif;
  }
  p {
    margin: 0;
    color: var(--text-2);
    font-size: 0.9375rem;
    line-height: 1.55;
  }
  .acts {
    display: flex;
    gap: 0.5rem;
    margin-top: 1.25rem;
  }
  button,
  .home {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 44px;
    padding: 0 1rem;
    border-radius: var(--r-md);
    border: 1px solid var(--line);
    background: var(--raised);
    color: var(--text);
    font:
      500 0.9375rem 'Lexend',
      sans-serif;
    text-decoration: none;
    cursor: pointer;
  }
  .reload {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
  }
  button:focus-visible,
  .home:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
</style>
