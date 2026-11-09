<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Layers, Library, Settings } from '@lucide/svelte';
  import { ApiError, createClient, type Entry, type Session } from '../lib/api/client';
  import HomeComposer from '../lib/components/HomeComposer.svelte';
  import ListRow from '../lib/components/ListRow.svelte';
  import Errbox from '../lib/components/Errbox.svelte';
  import SkeletonRows from '../lib/components/SkeletonRows.svelte';
  import TopBar from '../lib/components/TopBar.svelte';
  import { stashFirstMessage } from '../lib/state/handoff';
  import { draftsOf } from '../lib/state/lists';
  import { ageText, countText, entryMeta, waitingText } from '../lib/state/reading';

  /**
   * Home: the launcher. Drafts that are waiting come first, then what she has read lately, and the composer sits where
   * her thumb is. A first run is not an error state: it is an empty page that teaches by example.
   */
  const client = createClient();
  const thisYear = new Date().getFullYear();
  // The first-run examples are invented sentences, the first of them the one the dev world is scripted for, so the
  // whole flow can be tried offline by tapping it.
  const examples = [
    'finished the lantern keeper last night, 4.5, the ending landed but it was too long',
    'watched bramble hollow at the cinema, 5/5, the sound design carried it',
    'just beat hollow deep, 4 stars, the boss fights were great but the story dragged',
  ];

  let drafts = $state<Session[]>([]);
  let recent = $state<Entry[]>([]);
  let loading = $state(true);
  let problem = $state('');
  let text = $state('');
  let starting = $state(false);
  const now = new Date();

  const badge = $derived(drafts.length > 0 ? countText(drafts.length) : '');
  const firstRun = $derived(
    !loading && problem === '' && drafts.length === 0 && recent.length === 0,
  );

  async function load() {
    loading = true;
    problem = '';
    try {
      const [sessions, entries] = await Promise.all([
        client.listSessions({ state: 'active', kind: 'entry', limit: 100 }),
        client.listEntries({ limit: 4 }),
      ]);
      drafts = draftsOf(sessions.items);
      recent = entries.items;
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your library.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });

  async function start() {
    if (starting) return;
    // The sentence travels in memory, never in the URL: her words do not go in a browser's history.
    stashFirstMessage(text);
    starting = true;
    try {
      await goto(resolve('/new'));
    } finally {
      starting = false;
    }
  }
</script>

<svelte:head>
  <title>Sepia</title>
  <meta name="description" content="A private review journal for anything you finish." />
</svelte:head>

<div class="screen">
  <TopBar title="Sepia" wordmark>
    {#snippet actions()}
      <a
        class="iconbtn"
        href={resolve('/drafts')}
        aria-label={drafts.length === 1 ? 'Drafts, 1 waiting' : `Drafts, ${drafts.length} waiting`}
      >
        <Layers size={21} aria-hidden="true" />
        {#if badge}<span class="badge" aria-hidden="true">{badge}</span>{/if}
      </a>
      <a class="iconbtn" href={resolve('/library')} aria-label="Library">
        <Library size={21} aria-hidden="true" />
      </a>
      <a class="iconbtn" href={resolve('/settings')} aria-label="Settings">
        <Settings size={21} aria-hidden="true" />
      </a>
    {/snippet}
  </TopBar>

  <div class="scroll">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <p class="lbl"><span>Recent</span></p>
      <SkeletonRows rows={4} label="Reading your library" />
    {:else if firstRun}
      <div class="zero">
        <h2>Finished something?</h2>
        <p>
          Say what you thought, the way you would out loud. Sepia finds the details and asks a few
          questions.
        </p>
        <div class="ex">
          {#each examples as example (example)}
            <button type="button" onclick={() => (text = example)}>{example}</button>
          {/each}
        </div>
      </div>
    {:else}
      {#if drafts.length > 0}
        <p class="lbl">
          <span>Waiting for you</span>
          <a class="more" href={resolve('/drafts')}>All drafts</a>
        </p>
        <ul class="list">
          {#each drafts.slice(0, 3) as draft (draft.id)}
            <ListRow
              title={draft.title || 'Not identified yet'}
              waiting={waitingText(draft.waiting_reason)}
              age={ageText(draft.updated_at, now)}
              onclick={() => void goto(resolve('/s/[id]', { id: draft.id }))}
            />
          {/each}
        </ul>
      {/if}

      <p class="lbl">
        <span>Recent</span>
        <a class="more" href={resolve('/library')}>Library</a>
      </p>
      <ul class="list">
        {#each recent as entry (entry.id)}
          <ListRow
            title={entry.work.title}
            meta={entryMeta(entry, thisYear)}
            quarters={entry.rating_q}
            coverUrl={entry.work.cover_url}
            onclick={() => void goto(resolve('/entry/[id]', { id: entry.id }))}
          />
        {/each}
      </ul>
    {/if}
  </div>

  <HomeComposer bind:value={text} onsend={() => void start()} busy={starting} />
</div>

<style>
  .ex {
    display: grid;
    gap: 0.5rem;
    margin-top: 0.5rem;
    width: 100%;
  }
  .ex button {
    text-align: left;
    border: 1px solid var(--line);
    background: var(--panel);
    color: var(--text-2);
    border-radius: var(--r-md);
    padding: 0.7rem 0.85rem;
    font-size: 0.875rem;
    line-height: 1.45;
    min-height: 44px;
  }
  .ex button:hover {
    border-color: var(--accent);
    color: var(--text);
  }
</style>
