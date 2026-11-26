<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowDownWideNarrow, Clock, MessageCircle, Search, X } from '@lucide/svelte';
  import {
    ApiError,
    createClient,
    type Entry,
    type LibraryPage,
    type Medium,
    type SearchResults,
    type ShelfCounts,
  } from '../../lib/api/client';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import ListRow from '../../lib/components/ListRow.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { appendList, emptyList, replaceList, type ListState } from '../../lib/state/lists';
  import {
    completionBadge,
    entryMeta,
    sectionsFor,
    shelfEmpty,
    shelfOrder,
    SHELF_TABS,
    type Shelf,
  } from '../../lib/state/reading';
  import { mentionLine, splitSnippet } from '../../lib/state/search';

  /**
   * The Library: three shelves, one per tab, because "finished", "didn't finish" and "wanted" are three kinds of thing
   * rather than three filters over one list (docs/slices/S22-library-shelves.md). Finished groups by the month she
   * finished each thing; didn't finish groups by the month she stopped; wanted is one flat list ordered by when she
   * added it, since a wish has no date to group by. The medium chips filter the shelf in view, the tool row says how
   * many are in it and which order it is in, and a page is fifty entries with a way to ask for more.
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
  // The shelf in view. Finished opens first, which is the library as it was.
  let shelf = $state<Shelf>('finished');
  // The order. Only Finished has one to change, so the other shelves keep this at newest and never read it.
  let sort = $state<'newest' | 'rating'>('newest');
  // The number on each tab, loaded apart from the list so a count that failed leaves the tabs without a number rather
  // than the screen without a list. Null means "not known", which is not the same as zero.
  let counts = $state<ShelfCounts | null>(null);
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

  // A wish has no month she finished it and no rating to rank, so both of those lists are one flat section: the
  // headings would be dates she never reached, and the rating order would be an order of nothing.
  const groups = $derived(sectionsFor(list.items, sort, shelf === 'want_to' || sort === 'rating'));
  const order = $derived(shelfOrder(shelf, sort));
  const tabs = $derived(
    SHELF_TABS.map((tab) => ({
      id: tab.shelf,
      label: tab.label,
      count:
        counts === null
          ? undefined
          : tab.shelf === 'finished'
            ? counts.finished
            : tab.shelf === 'abandoned'
              ? counts.abandoned
              : counts.want_to,
    })),
  );
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
  const empty = $derived(shelfEmpty(shelf, chosen === '' ? null : { chosen }));

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
        // Always a shelf: the API's omitted default is the old combined list, which no tab shows.
        completion: shelf,
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

  // The counts are read once, not per page: they are the whole table's totals, so a filter or a second page cannot
  // change them. A failure here is silent on purpose, and the tabs simply carry no number.
  async function loadCounts() {
    try {
      counts = await client.shelfCounts();
    } catch {
      counts = null;
    }
  }
  void loadCounts();

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

  // Switching shelf reads its own first page, and leaves the order where a shelf can hold it: a rating order on the
  // other two shelves would be an order of rows that have no rating.
  function chooseShelf(next: string) {
    shelf = next as Shelf;
    if (shelf !== 'finished') sort = 'newest';
  }

  function flipSort() {
    sort = sort === 'newest' ? 'rating' : 'newest';
  }
</script>

<svelte:head>
  <title>Library · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Library" back>
    {#snippet actions()}
      <!-- The way into the chat: it is the only door to it, so it lives where the mockup puts it. -->
      <a class="ask" href={resolve('/chat')} aria-label="Ask your library">
        <MessageCircle size={20} aria-hidden="true" />
      </a>
    {/snippet}
  </TopBar>

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
      <Tabs {tabs} selected={shelf} label="Shelf" showZero onselect={chooseShelf} />

      <div class="chipsrow" role="group" aria-label="Filter by medium">
        {#each media as option (option.value)}
          <Chip pressed={medium === option.value} onclick={() => chooseMedium(option.value)}>
            {option.label}
          </Chip>
        {/each}
      </div>

      <div class="toolrow">
        <span>{countLine}</span>
        {#if order.toggle && list.items.length > 0}
          <button type="button" class="linkbtn" onclick={flipSort} aria-label="Sort: {order.label}">
            <ArrowDownWideNarrow size={16} aria-hidden="true" />
            {order.label}
          </button>
        {:else if list.items.length > 0}
          <!-- Nothing to rank on this shelf, so its order is a fact rather than a control. -->
          <span class="fixed" aria-label="Order: {order.label}">
            <Clock size={16} aria-hidden="true" />
            {order.label}
          </span>
        {/if}
      </div>

      {#if problem}
        <Errbox message={problem} retryable onretry={() => void load(true)} />
      {:else if loading}
        <SkeletonRows label="Reading your library" />
      {:else if emptyFilter}
        <div class="zero">
          <h2>{empty.heading}</h2>
          <p>{empty.body}</p>
          {#if empty.action === 'Show everything'}
            <Button onclick={() => chooseMedium('')}>Show everything</Button>
          {:else}
            <Button variant="primary" onclick={() => void goto(resolve('/new'))}>
              {empty.action}
            </Button>
          {/if}
        </div>
      {:else}
        {#each groups as group (group.key)}
          {#if group.label}
            <p class="grp" id="grp-{group.key || 'none'}">{group.label}</p>
          {/if}
          <ul
            class="list"
            id="panel-{shelf}"
            aria-labelledby={group.label ? `grp-${group.key || 'none'}` : `tab-${shelf}`}
          >
            {#each group.items as entry (entry.id)}
              <!-- A stopped row has no right-hand slot and no stars unless she gave one, and its how-far line is the
                   second line in her own words, in quotes. A wanted row has no stars at all. Neither is on its shelf
                   with a badge: the tab above already says which list this is (mockup 03, frames 5 and 6). -->
              <ListRow
                title={entry.work.title}
                meta={entryMeta(entry, thisYear)}
                quarters={entry.rating_q > 0 ? entry.rating_q : null}
                note={entry.completion === 'abandoned' && entry.progress
                  ? `\u201c${entry.progress}\u201d`
                  : undefined}
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
  .ask {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    border-radius: 999px;
    color: var(--text-2);
  }
  .ask:hover {
    color: var(--text);
  }
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
  .sortlabel,
  .fixed {
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
