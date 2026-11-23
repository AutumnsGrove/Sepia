<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowDownWideNarrow, Search, X } from '@lucide/svelte';
  import {
    ApiError,
    createClient,
    type Entry,
    type LibraryPage,
    type Medium,
    type SearchResults,
  } from '../../lib/api/client';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { appendList, emptyList, replaceList, type ListState } from '../../lib/state/lists';
  import { completionBadge, entryMeta, sectionsFor } from '../../lib/state/reading';
  import { mentionLine, splitSnippet } from '../../lib/state/search';

  /**
   * The Library: one list across every medium, grouped by the month she finished each thing. The medium chips filter
   * it, the tool row says how many there are and which way they are sorted, and a page is fifty entries with a way to
   * ask for more.
   *
   * Search reads her own words: the work's title, the review she wrote and the answers she gave. Typing replaces the
   * grouped list with ranked results and the sentence the match was found in, and clearing the box puts the library
   * back exactly where it was. The query is debounced because every keystroke is a round trip, and a later search can
   * never overwrite an earlier one's results.
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
  // The shelf. Empty is the library proper: finished and abandoned reviews. Wanted is one chip away, because a work
  // she has not read has no finish date and is not a review (docs/slices/S21-wishlist-and-unfinished.md).
  let shelf = $state<'' | 'want_to'>('');
  let sort = $state<'newest' | 'rating'>('newest');
  let list = $state<ListState<Entry>>(emptyList<Entry>());
  let loading = $state(true);
  let loadingMore = $state(false);
  let problem = $state('');
  let loaded = $state(false);
  // Search is its own read: the list below and the results are never shown at the same time.
  let query = $state('');
  let results = $state<SearchResults | null>(null);
  let searching = $state(false);
  let searchProblem = $state('');
  let searchCount = $state(0);

  const groups = $derived(sectionsFor(list.items, sort, shelf === 'want_to'));
  const sortLabel = $derived(sort === 'newest' ? 'Newest first' : 'Highest rated');
  // The total is the server's count of everything the filters hold, not the rows on the page she is reading. Before
  // the first page arrives it falls back to what is on screen.
  const countLine = $derived(
    list.total > 0
      ? `${list.total} ${list.total === 1 ? 'entry' : 'entries'}`
      : list.items.length > 0
        ? `${list.items.length} ${list.items.length === 1 ? 'entry' : 'entries'}`
        : '',
  );
  const searchLine = $derived(mentionLine(searchCount, query));
  const searchHits = $derived(results?.items ?? []);
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
        completion: shelf === '' ? undefined : shelf,
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
    // Any change of filter or shelf or order starts again from the first page: a cursor belongs to the query that
    // made it. Each is read on its own line rather than in a short-circuiting expression, so the effect is registered
    // against all three and not only against the first truthy one.
    void medium;
    void shelf;
    void sort;
    // While a search is showing, the list behind it is not needed. Clearing the box runs this again.
    if (query.trim() !== '') return;
    list = emptyList<Entry>();
    void load(true);
  });

  // A search is debounced: 250 ms is long enough that typing a word is one request, and short enough that the screen
  // still feels like it is answering. Each run carries a number so a slow earlier run cannot replace a newer one.
  let searchSeq = 0;
  $effect(() => {
    const text = query.trim();
    if (text === '') {
      searchSeq++;
      results = null;
      searching = false;
      searchProblem = '';
      searchCount = 0;
      return;
    }
    const mine = ++searchSeq;
    searching = true;
    searchProblem = '';
    const handle = setTimeout(() => void runSearch(text, mine), 250);
    return () => clearTimeout(handle);
  });

  async function runSearch(text: string, mine: number) {
    try {
      const found = await client.search(text);
      if (mine !== searchSeq) return;
      results = found;
      searchCount = found.total;
    } catch (error) {
      if (mine !== searchSeq) return;
      searchProblem =
        error instanceof ApiError ? error.copy : 'Sepia could not search your library just now.';
    } finally {
      if (mine === searchSeq) searching = false;
    }
  }

  function clearSearch() {
    query = '';
  }

  function retrySearch() {
    const text = query.trim();
    if (text === '') return;
    searchSeq++;
    const mine = searchSeq;
    searching = true;
    searchProblem = '';
    void runSearch(text, mine);
  }

  const searchOpen = $derived(query.trim() !== '');

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
      <!-- A text box with a search hint rather than type="search": the browser's own clear button would sit beside
           the one this screen draws, and two Xs in one field is one too many. -->
      <input
        type="text"
        role="searchbox"
        enterkeyhint="search"
        placeholder="Search titles and your words"
        aria-label="Search titles and your words"
        bind:value={query}
      />
      {#if query !== ''}
        <button type="button" class="clear" aria-label="Clear search" onclick={clearSearch}>
          <X size={16} aria-hidden="true" />
        </button>
      {/if}
    </label>

    {#if searchOpen}
      {#if searching || searchHits.length > 0}
        <div class="toolrow" aria-live="polite">
          <span>{searching && !results ? 'Searching' : searchLine}</span>
          <span class="sortlabel" aria-label="Order: best match">
            <ArrowDownWideNarrow size={16} aria-hidden="true" />
            Best match
          </span>
        </div>
      {/if}
      {#if searchProblem}
        <Errbox message={searchProblem} retryable onretry={retrySearch} />
      {:else if searching && !results}
        <SkeletonRows label="Searching your words" />
      {:else if searchHits.length === 0}
        <div class="zero">
          <h2>Nothing mentions “{query.trim()}”.</h2>
          <p>
            Search reads the titles, the reviews you wrote and the answers you gave. Try a word you
            remember writing, or a shorter one.
          </p>
          <Button onclick={clearSearch}>Back to the library</Button>
        </div>
      {:else}
        <ul class="list" aria-label="Search results">
          {#each searchHits as hit (hit.id)}
            {#if hit.entry}
              <ListRow
                title={hit.entry.work.title}
                meta={entryMeta(hit.entry, thisYear)}
                quarters={hit.entry.rating_q > 0 ? hit.entry.rating_q : null}
                badge={completionBadge(hit.entry)}
                medium={hit.entry.work.medium}
                favourite={hit.entry.favourite}
                coverUrl={hit.entry.work.cover_url}
                noteSegments={splitSnippet(hit.snippet)}
                onclick={() => void goto(resolve('/entry/[id]', { id: hit.id }))}
              />
            {/if}
          {/each}
        </ul>
        {#if searchCount > searchHits.length}
          <p class="more">
            Showing {searchHits.length} of {searchCount}. Add a word to narrow it.
          </p>
        {/if}
      {/if}
    {:else}
      <div class="chipsrow" role="group" aria-label="Filter by medium">
        {#each media as option (option.value)}
          <Chip pressed={medium === option.value} onclick={() => chooseMedium(option.value)}>
            {option.label}
          </Chip>
        {/each}
      </div>

      <div class="chipsrow" role="group" aria-label="Filter by shelf">
        <Chip pressed={shelf === ''} onclick={() => (shelf = '')}>Reviewed</Chip>
        <Chip pressed={shelf === 'want_to'} onclick={() => (shelf = 'want_to')}>Wanted</Chip>
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
          <h2>
            {shelf === 'want_to'
              ? 'Nothing on your wanted list yet.'
              : medium === ''
                ? 'Nothing here yet.'
                : `No ${chosen.toLowerCase()} entries yet.`}
          </h2>
          <p>
            {shelf === 'want_to'
              ? 'Say what you mean to read, watch or play and it lands here, with no rating and nothing to write yet.'
              : medium === ''
                ? 'Everything you finish and write about lands here, across films, books, games, albums and more.'
                : 'Nothing in your library yet, with this filter on.'}
          </p>
          {#if shelf === 'want_to'}
            <Button variant="primary" onclick={() => void goto(resolve('/new'))}
              >Add something</Button
            >
          {:else if medium === ''}
            <Button variant="primary" onclick={() => void goto(resolve('/new'))}
              >Log something</Button
            >
          {:else}
            <Button onclick={() => chooseMedium('')}>Show everything</Button>
          {/if}
        </div>
      {:else}
        {#each groups as group (group.key)}
          {#if group.label}
            <p class="grp" id="grp-{group.key || 'none'}">{group.label}</p>
          {/if}
          <ul class="list" aria-labelledby={group.label ? `grp-${group.key || 'none'}` : undefined}>
            {#each group.items as entry (entry.id)}
              <ListRow
                title={entry.work.title}
                meta={entryMeta(entry, thisYear)}
                quarters={entry.rating_q > 0 ? entry.rating_q : null}
                badge={completionBadge(entry)}
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
    {/if}
  </div>
</main>

<style>
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
  .search:focus-within {
    border-color: var(--accent);
  }
  .search input {
    flex: 1;
    min-width: 0;
    min-height: 44px;
    border: 0;
    background: none;
    outline: none;
    color: var(--text);
    font-size: 0.9375rem;
  }
  .search input::placeholder {
    color: var(--text-3);
  }
  /* The clear button sits inside the field's own height, so it is a target without making the box taller. */
  .clear {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    margin-right: -0.5rem;
    padding: 0;
    border: 0;
    background: none;
    border-radius: 999px;
    color: var(--text-3);
  }
  .clear:hover {
    color: var(--text);
  }
  /* The order in search is always relevance, so it is a label rather than a button: nothing to toggle. */
  .sortlabel {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-2);
    font-size: 0.8125rem;
  }
  .more {
    margin: 0.9rem 0 0;
    color: var(--text-3);
    font-size: 0.8125rem;
    text-align: center;
  }
</style>
