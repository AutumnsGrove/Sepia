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
  import {
    ageText,
    completionBadge,
    countText,
    entryMeta,
    waitingText,
  } from '../lib/state/reading';

  /**
   * Home: the launcher. Drafts that are waiting come first, then what she has read lately, and the composer sits where
   * her thumb is. A first run is not an error state: it is an empty page that asks the question and explains what
   * happens next, and then stops talking. A suggested review would be the app putting words in her mouth.
   */
  const client = createClient();
  const thisYear = new Date().getFullYear();

  let drafts = $state<Session[]>([]);
  let recent = $state<Entry[]>([]);
  let loading = $state(true);
  let problem = $state('');
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

  async function start(sentence: string) {
    if (starting) return;
    // The sentence travels in memory, never in the URL: her words do not go in a browser's history.
    stashFirstMessage(sentence);
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

<main class="screen">
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
              noCover
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
            quarters={entry.rating_q > 0 ? entry.rating_q : null}
            badge={completionBadge(entry)}
            favourite={entry.favourite}
            medium={entry.work.medium}
            coverUrl={entry.work.cover_url}
            onclick={() => void goto(resolve('/entry/[id]', { id: entry.id }))}
          />
        {/each}
      </ul>
    {/if}
  </div>

  <HomeComposer onsend={(sentence) => void start(sentence)} busy={starting} />
</main>
