<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, CircleAlert, Minus, RotateCcw } from '@lucide/svelte';
  import { ApiError, createClient, type ServicesInfo } from '../../../lib/api/client';
  import Errbox from '../../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../../lib/components/TopBar.svelte';
  import { serviceRows } from '../../../lib/update/services';

  /**
   * Services: what Sepia depends on and what stops working when one is missing. The route makes no live call, so the
   * word is "Ready" (configured) rather than "Reachable": a screen that explains a slow dependency must not become
   * one. Check again re-reads the same answer, which is what changes when the container is recreated.
   */
  const client = createClient();

  let info = $state<ServicesInfo | null>(null);
  let loading = $state(true);
  let problem = $state('');
  let checking = $state(false);

  const rows = $derived(serviceRows(info));

  async function load() {
    loading = true;
    problem = '';
    try {
      info = await client.getServices();
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read the services.';
    } finally {
      loading = false;
    }
  }

  async function again() {
    checking = true;
    try {
      info = await client.getServices();
      problem = '';
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read the services.';
    } finally {
      checking = false;
    }
  }

  $effect(() => {
    void load();
  });
</script>

<svelte:head>
  <title>Services · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Services" back>
    {#snippet actions()}
      <button
        type="button"
        class="iconbtn"
        aria-label="Check again"
        disabled={checking}
        onclick={() => void again()}
      >
        <RotateCcw size={20} aria-hidden="true" />
      </button>
    {/snippet}
  </TopBar>

  <div class="scroll">
    {#if loading}
      <SkeletonRows rows={5} label="Reading the services" />
    {:else if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else}
      <p class="lede">
        Everything Sepia depends on, and what stops working if it is down. One unreachable service
        never blocks the rest.
      </p>
      <ul class="svcs">
        {#each rows as row (row.name)}
          <li
            class="svc"
            class:bad={row.state === 'down' || row.state === 'degraded'}
            class:unset={row.state === 'not_configured'}
          >
            {#if row.state === 'ok'}
              <Check size={18} aria-hidden="true" />
            {:else if row.state === 'not_configured'}
              <Minus size={18} aria-hidden="true" />
            {:else}
              <CircleAlert size={18} aria-hidden="true" />
            {/if}
            <div>
              <p class="nm">{row.label}</p>
              <p class="dt">{row.state !== 'ok' && row.cost ? row.cost : row.detail}</p>
            </div>
            <span class="rt2">{row.stateText}</span>
          </li>
        {/each}
      </ul>
      <p class="fine">
        This page says what is set up. It does not call anything: a slow dependency should never
        make the screen that explains it slow. <a href={resolve('/settings/updates')}>Updates</a> has
        the helper's state.
      </p>
    {/if}
  </div>
</main>

<style>
  .lede {
    margin: 0.4rem 0 0.8rem;
    color: var(--text-2);
    font-size: 0.9375rem;
    line-height: 1.5;
  }
  .svcs {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .svc {
    display: grid;
    grid-template-columns: 22px 1fr auto;
    gap: 0.75rem;
    align-items: start;
    padding: 0.85rem 0;
    border-bottom: 1px solid var(--line);
  }
  .svc:last-child {
    border-bottom: 0;
  }
  .svc :global(svg) {
    margin-top: 2px;
    color: var(--accent);
  }
  .svc.bad :global(svg) {
    color: var(--danger);
  }
  .svc.unset :global(svg) {
    color: var(--text-3);
  }
  .nm {
    margin: 0;
    font-size: 1rem;
    color: var(--text);
  }
  .dt {
    margin: 0.15rem 0 0;
    font-size: 0.8125rem;
    color: var(--text-2);
    line-height: 1.45;
  }
  .rt2 {
    font-size: 0.8125rem;
    color: var(--text-3);
    white-space: nowrap;
  }
  .svc.bad .rt2 {
    color: var(--danger);
  }
  .svc.unset .rt2 {
    color: var(--text-3);
  }
  .fine {
    margin: 1.4rem 0 0;
    font-size: 0.8125rem;
    color: var(--text-3);
    line-height: 1.5;
  }
  .fine a {
    color: var(--accent);
  }
</style>
