<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowDownWideNarrow, Search } from '@lucide/svelte';
  import {
    ApiError,
    createClient,
    type Entry,
    type LibraryPage,
    type Medium,
  } from '../../lib/api/client';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { appendList, emptyList, replaceList, type ListState } from '../../lib/state/lists';
  import { entryMeta, groupByMonth } from '../../lib/state/reading';

  /**
   * The Library: one list across every medium, grouped by the month she finished each thing. The medium chips filter
   * it, the tool row says how many there are and which way they are sorted, and a page is fifty entries with a way to
   * ask for more.
   *
   * Search's place is reserved but not pretending: the box is there and disabled, because a search that did nothing
   * would be worse than one that is not built (S16).
   */
  const client = createClient();
  const thisYear = new Date().getFullYear();
  const media: { value: Medium | ''; label: string }[] = [
    { value: '', label: 'All' },
    { value: 'film', label: 'Film' },
    { value: 'tv', label: 'TV' },
    { value: 'book', label: 'Book' },
    { value: 'game', label: 'Game' },
    { value: 'album', label: 'Album' },
    { value: 'podcast', label: 'Podcast' },
    { value: 'theatre', label: 'Theatre' },
  ];

  let medium = $state<Medium | ''>('');
  let sort = $state<'newest' | 'rating'>('newest');
  let list = $state<ListState<Entry>>(emptyList<Entry>());
  let loading = $state(true);
  let loadingMore = $state(false);
  let problem = $state('');
  let loaded = $state(false);

  const groups = $derived(groupByMonth(list.items));
  const sortLabel = $derived(sort === 'newest' ? 'Newest first' : 'Highest rated');
  const countLine = $derived(
    list.items.length === 0
      ? ''
      : `${list.items.length}${list.hasMore ? '+' : ''} ${list.items.length === 1 ? 'entry' : 'entries'}`,
  );
  const emptyFilter = $derived(
    !loading && problem === '' && list.items.length === 0 && (medium !== '' || loaded),
  );
  const chosen = $derived(media.find((option) => option.value === medium)?.label ?? '');

  async function load(reset: boolean) {
    if (reset) {
      loading = true;
      problem = '';
    } else {
      loadingMore = true;
    }
    try {
      const page: LibraryPage = await client.listEntries({
        medium: medium === '' ? undefined : medium,
        sort,
        cursor: reset ? undefined : (list.cursor ?? undefined),
        limit: 50,
      });
      list = reset ? replaceList(page) : appendList(list, page, (entry) => entry.id);
      loaded = true;
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your library.';
    } finally {
      loading = false;
      loadingMore = false;
    }
  }

  $effect(() => {
    // Any change of filter or order starts again from the first page: a cursor belongs to the query that made it.
    void (medium && sort);
    list = emptyList<Entry>();
    void load(true);
  });

  function chooseMedium(value: Medium | '') {
    medium = value;
  }

  function flipSort() {
    sort = sort === 'newest' ? 'rating' : 'newest';
  }
</script>

<svelte:head>
  <title>Library · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Library" back />

  <div class="scroll">
    <label class="search">
      <Search size={18} aria-hidden="true" />
      <input
        type="search"
        placeholder="Search titles and your words"
        aria-label="Search titles and your words (arrives later)"
        disabled
      />
    </label>

    <div class="chipsrow" role="group" aria-label="Filter by medium">
      {#each media as option (option.value)}
        <Chip pressed={medium === option.value} onclick={() => chooseMedium(option.value)}>
          {option.label}
        </Chip>
      {/each}
    </div>

    <div class="toolrow">
      <span>{countLine}</span>
      {#if list.items.length > 0}
        <button type="button" class="linkbtn" onclick={flipSort} aria-label="Sort: {sortLabel}">
          <ArrowDownWideNarrow size={16} aria-hidden="true" />
          {sortLabel}
        </button>
      {/if}
    </div>

    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load(true)} />
    {:else if loading}
      <SkeletonRows label="Reading your library" />
    {:else if emptyFilter}
      <div class="zero">
        <h2>{medium === '' ? 'Nothing here yet.' : `No ${chosen.toLowerCase()} entries yet.`}</h2>
        <p>
          {medium === ''
            ? 'Everything you finish and write about lands here, across films, books, games, albums and more.'
            : 'Nothing in your library yet, with this filter on.'}
        </p>
        {#if medium === ''}
          <Button variant="primary" onclick={() => void goto(resolve('/new'))}>Log something</Button
          >
        {:else}
          <Button onclick={() => chooseMedium('')}>Show everything</Button>
        {/if}
      </div>
    {:else}
      {#each groups as group (group.key)}
        <p class="grp" id="grp-{group.key || 'none'}">{group.label}</p>
        <ul class="list" aria-labelledby="grp-{group.key || 'none'}">
          {#each group.items as entry (entry.id)}
            <ListRow
              title={entry.work.title}
              meta={entryMeta(entry, thisYear)}
              quarters={entry.rating_q}
              medium={entry.work.medium}
              favourite={entry.favourite}
              coverUrl={entry.work.cover_url}
              onclick={() => void goto(resolve('/entry/[id]', { id: entry.id }))}
            />
          {/each}
        </ul>
      {/each}
      {#if list.hasMore}
        <div class="btnrow">
          <Button grow disabled={loadingMore} onclick={() => void load(false)}>
            {loadingMore ? 'Loading' : 'More entries'}
          </Button>
        </div>
      {/if}
    {/if}
  </div>
</main>

<style>
  /* The search box's place, reserved: it is a control and it says why it cannot be used yet. */
  .search {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 44px;
    margin-top: 0.35rem;
    padding: 0 0.85rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--panel);
    color: var(--text-3);
  }
  .search input {
    flex: 1;
    min-height: 44px;
    border: 0;
    background: none;
    outline: none;
    color: var(--text-3);
    font-size: 0.9375rem;
  }
</style>
