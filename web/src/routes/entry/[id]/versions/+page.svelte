<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import {
    ApiError,
    createClient,
    type Entry,
    type RevisionPage,
  } from '../../../../lib/api/client';
  import Button from '../../../../lib/components/Button.svelte';
  import Errbox from '../../../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../../../lib/components/TopBar.svelte';
  import { versionRows } from '../../../../lib/state/reading';

  /**
   * Earlier versions: every confirmed edit, newest first, with the version she has now at the top. Nothing was ever
   * overwritten and nothing was ever deleted, so this is the whole history rather than a summary of it.
   *
   * Restore as draft opens the old version as a new working copy: confirming it makes that version the newest one and
   * keeps the version it replaced, so going back is one more step and never a loss.
   */
  const client = createClient();
  const entryID = $derived(page.params.id ?? '');

  let entry = $state<Entry | null>(null);
  let revisions = $state<RevisionPage['items']>([]);
  let loading = $state(true);
  let problem = $state('');
  let missing = $state(false);

  const rows = $derived(entry ? versionRows(entry, revisions) : []);

  async function load() {
    loading = true;
    problem = '';
    missing = false;
    try {
      entry = await client.getEntry(entryID);
      revisions = (await client.listRevisions(entryID)).items;
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) missing = true;
      else
        problem = error instanceof ApiError ? error.copy : 'Sepia could not read these versions.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (entryID) void load();
  });

  function restoreDraft(number: number) {
    if (!entry) return;
    const query = `reopen=${encodeURIComponent(entry.id)}&revision=${String(number)}`;
    // eslint-disable-next-line svelte/no-navigation-without-resolve
    void goto(`${resolve('/new')}?${query}`);
  }

  function when(iso: string): string {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('en-GB', {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: 'UTC',
    }).format(date);
  }
</script>

<svelte:head>
  <title>Earlier versions · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Earlier versions" back />

  <div class="scroll">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <SkeletonRows rows={3} label="Reading the versions" />
    {:else if missing || !entry}
      <div class="zero">
        <h2>This entry is not here.</h2>
        <p>The link may be old, or the entry may never have existed.</p>
        <Button onclick={() => void goto(resolve('/library'))}>Back to the library</Button>
      </div>
    {:else}
      <p class="note">
        Nothing is overwritten. {rows.length === 1
          ? 'This entry has one version.'
          : `${entry.work.title} has ${rows.length} versions.`}
      </p>
      {#each rows as row (row.number)}
        <div class="ver">
          <div class="vh">
            <b>
              Version {row.number}
              {#if row.current}<span class="cur">Current</span>{/if}
            </b>
            <span class="when">{when(row.at)}</span>
          </div>
          <p class="ch">{row.change}</p>
          <p class="words">{row.entry.body}</p>
          {#if !row.current}
            <div class="btnrow">
              <Button size="sm" onclick={() => restoreDraft(row.number)}>Restore as draft</Button>
            </div>
          {/if}
        </div>
      {/each}
    {/if}
  </div>
</main>

<style>
  .note {
    margin: 0.5rem 0 1rem;
    font-size: 0.9375rem;
    color: var(--text-2);
  }
  .ver {
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--panel);
    padding: 0.85rem 0.95rem;
    margin-bottom: 0.7rem;
  }
  .vh {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .vh b {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    font: 500 0.9375rem 'Lexend';
    color: var(--text);
  }
  .cur {
    padding: 0.1rem 0.45rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font: 500 0.6875rem 'Lexend';
  }
  .when {
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .ch {
    margin: 0.4rem 0 0;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .words {
    margin: 0.55rem 0 0;
    font-size: 0.9375rem;
    line-height: 1.6;
    color: var(--text);
    overflow-wrap: anywhere;
  }
  .btnrow {
    margin-top: 0.7rem;
  }
</style>
