<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { RotateCcw } from '@lucide/svelte';
  import { ApiError, createClient, type Entry } from '../../lib/api/client';
  import Button from '../../lib/components/Button.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { deletedMeta } from '../../lib/state/reading';

  /**
   * Deleted: hidden, never erased. This is the only list they appear in, and Restore is the only way out. Restoring is
   * not a rebuild, because nothing was taken away: the row is un-marked and the entry is back where it was.
   */
  const client = createClient();
  const thisYear = new Date().getFullYear();

  let items = $state<Entry[]>([]);
  let loading = $state(true);
  let problem = $state('');
  let restoring = $state<string | null>(null);

  async function load() {
    loading = true;
    problem = '';
    try {
      const page = await client.listEntries({ deleted: true, limit: 100 });
      items = page.items;
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read the Deleted list.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });

  async function restore(entry: Entry) {
    if (restoring) return;
    restoring = entry.id;
    try {
      await client.restoreEntry(entry.id);
      items = items.filter((row) => row.id !== entry.id);
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not restore that entry.';
    } finally {
      restoring = null;
    }
  }
</script>

<svelte:head>
  <title>Deleted · Sepia</title>
</svelte:head>

<div class="screen">
  <TopBar title="Deleted" back />

  <div class="scroll">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <SkeletonRows rows={3} label="Reading the Deleted list" />
    {:else if items.length === 0}
      <div class="zero">
        <h2>Nothing deleted.</h2>
        <p>
          An entry you delete from its own page waits here, out of your library and still in your
          backups.
        </p>
        <Button onclick={() => void goto(resolve('/library'))}>Back to the library</Button>
      </div>
    {:else}
      <p class="note">
        Deleted entries stay in your backups. This list is the only place they show up.
      </p>
      <ul class="list">
        {#each items as entry (entry.id)}
          <li class="row">
            <ListRow
              title={entry.work.title}
              meta={deletedMeta(entry, thisYear)}
              coverUrl={entry.work.cover_url}
              onclick={() => void goto(resolve('/entry/[id]', { id: entry.id }))}
            />
            <Button size="sm" disabled={restoring === entry.id} onclick={() => void restore(entry)}>
              <RotateCcw size={14} aria-hidden="true" />
              {restoring === entry.id ? 'Restoring' : 'Restore'}
              <span class="sr">{entry.work.title}</span>
            </Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .note {
    margin: 0.5rem 0 0.75rem;
    font-size: 0.875rem;
    color: var(--text-2);
  }
  .row {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.5rem;
    align-items: center;
    border-bottom: 1px solid var(--line);
  }
  .row :global(.row) {
    border-bottom: 0;
  }
</style>
