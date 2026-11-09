<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ApiError, createClient, type Session } from '../../lib/api/client';
  import Errbox from '../../lib/components/Errbox.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { draftsOf } from '../../lib/state/lists';
  import { ageText, waitingText } from '../../lib/state/reading';

  /**
   * Drafts: the cards that are waiting, each saying what it is waiting for. They never expire, so this list is the
   * promise that nothing she started is lost. Tapping a row goes back to its card, which is where it left off.
   */
  const client = createClient();
  const now = new Date();

  let drafts = $state<Session[]>([]);
  let loading = $state(true);
  let problem = $state('');

  async function load() {
    loading = true;
    problem = '';
    try {
      const page = await client.listSessions({ state: 'active', kind: 'entry', limit: 100 });
      drafts = draftsOf(page.items);
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your drafts.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });
</script>

<svelte:head>
  <title>Drafts · Sepia</title>
</svelte:head>

<div class="screen">
  <TopBar title="Drafts" back />

  <div class="scroll">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <SkeletonRows label="Reading your drafts" />
    {:else if drafts.length === 0}
      <div class="zero">
        <h2>Nothing waiting.</h2>
        <p>An unfinished review waits here until you save it or throw it away. Nothing expires.</p>
      </div>
    {:else}
      <ul class="list">
        {#each drafts as draft (draft.id)}
          <ListRow
            title={draft.title || 'Not identified yet'}
            waiting={waitingText(draft.waiting_reason)}
            age={ageText(draft.updated_at, now)}
            onclick={() => void goto(resolve('/s/[id]', { id: draft.id }))}
          />
        {/each}
      </ul>
    {/if}
  </div>
</div>
