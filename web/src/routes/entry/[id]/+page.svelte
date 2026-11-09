<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { Check, ChevronDown, History, RotateCcw } from '@lucide/svelte';
  import {
    ApiError,
    createClient,
    type Entry,
    type RevisionPage,
    type WorkDetail,
  } from '../../../lib/api/client';
  import Button from '../../../lib/components/Button.svelte';
  import Cover from '../../../lib/components/Cover.svelte';
  import EntryMenu from '../../../lib/components/EntryMenu.svelte';
  import Errbox from '../../../lib/components/Errbox.svelte';
  import Pill from '../../../lib/components/Pill.svelte';
  import Rating from '../../../lib/components/Rating.svelte';
  import SkeletonRows from '../../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../../lib/components/TopBar.svelte';
  import { dateText, mediumText, ratingText } from '../../../lib/session/format';
  import {
    describeGroups,
    otherEntries,
    questionAnswers,
    shortDate,
    warningsOf,
    deletedMeta,
  } from '../../../lib/state/reading';

  /**
   * A saved entry: her words first, the details quietly below, and the way to edit, log again, go back through the
   * versions, or delete it without erasing it.
   *
   * A deleted entry is still readable and still says what it is, because a link followed after a delete is how she
   * reaches Restore. An id that never existed is a plain not-found state, not an empty screen.
   */
  const client = createClient();
  const thisYear = new Date().getFullYear();
  const entryID = $derived(page.params.id ?? '');

  let entry = $state<Entry | null>(null);
  let work = $state<WorkDetail | null>(null);
  let revisions = $state<RevisionPage['items']>([]);
  let onWork = $state<Entry[]>([]);
  let loading = $state(true);
  let problem = $state('');
  let missing = $state(false);
  let confirming = $state(false);
  let deleting = $state(false);
  let restoring = $state(false);

  const tags = $derived(entry?.tags ?? []);
  const groupChips = $derived(describeGroups(entry?.answers ?? []));
  const questions = $derived(questionAnswers(entry?.answers ?? []));
  const warnings = $derived(warningsOf(entry?.answers ?? []));
  const others = $derived(entry ? otherEntries(onWork, entry.id) : []);
  const provider = $derived(
    (work?.primary_provider ?? entry?.work.primary_provider ?? '') === 'tmdb'
      ? 'TMDB'
      : (work?.primary_provider ?? entry?.work.primary_provider ?? '') === 'openlibrary'
        ? 'Open Library'
        : (work?.primary_provider ?? ''),
  );

  async function load() {
    loading = true;
    problem = '';
    missing = false;
    try {
      const found = await client.getEntry(entryID);
      entry = found;
      // The history and the work's own detail are useful but not essential: a failure there must not blank the page.
      revisions = await client
        .listRevisions(entryID)
        .then((pageOfRevisions) => pageOfRevisions.items)
        .catch(() => []);
      work = await client.getWork(found.work.id).catch(() => null);
      const workEntries = await client
        .listEntries({ work: found.work.id, limit: 20 })
        .then((pageOfEntries) => pageOfEntries.items)
        .catch(() => []);
      onWork = workEntries;
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) missing = true;
      else problem = error instanceof ApiError ? error.copy : 'Sepia could not read this entry.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (entryID) void load();
  });

  async function remove() {
    if (!entry || deleting) return;
    deleting = true;
    try {
      await client.deleteEntry(entry.id);
      await goto(resolve('/deleted'));
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not delete this entry.';
      confirming = false;
    } finally {
      deleting = false;
    }
  }

  async function restore() {
    if (!entry || restoring) return;
    restoring = true;
    try {
      await client.restoreEntry(entry.id);
      await load();
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not restore this entry.';
    } finally {
      restoring = false;
    }
  }

  // The query is a variant of the same route, so it is added to resolve()'s path rather than built by hand.
  function edit() {
    // eslint-disable-next-line svelte/no-navigation-without-resolve
    if (entry) void goto(`${resolve('/new')}?reopen=${encodeURIComponent(entry.id)}`);
  }

  function again() {
    // eslint-disable-next-line svelte/no-navigation-without-resolve
    if (entry) void goto(`${resolve('/new')}?reopen=${encodeURIComponent(entry.id)}&mode=new`);
  }

  function versions() {
    if (entry) void goto(resolve('/entry/[id]/versions', { id: entry.id }));
  }
</script>

<svelte:head>
  <title>{entry?.work.title ?? 'Entry'} · Sepia</title>
</svelte:head>

<div class="screen">
  <TopBar title="Entry" back>
    {#snippet actions()}
      {#if entry}
        <EntryMenu
          onedit={edit}
          onagain={again}
          onversions={versions}
          ondelete={() => (confirming = true)}
          deleted={Boolean(entry.deleted_at)}
          onrestore={() => void restore()}
        />
      {/if}
    {/snippet}
  </TopBar>

  <div class="scroll page">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <SkeletonRows rows={4} label="Reading this entry" />
    {:else if missing || !entry}
      <div class="zero">
        <h2>This entry is not here.</h2>
        <p>The link may be old, or the entry may never have existed.</p>
        <Button onclick={() => void goto(resolve('/library'))}>Back to the library</Button>
      </div>
    {:else}
      {#if entry.deleted_at}
        <div class="deletednote" role="status">
          <p>
            <b>{deletedMeta(entry, thisYear)}.</b> It is out of your library, and nothing is erased.
          </p>
          <Button size="sm" disabled={restoring} onclick={() => void restore()}>
            <RotateCcw size={14} aria-hidden="true" />
            {restoring ? 'Restoring' : 'Restore'}
          </Button>
        </div>
      {/if}

      <section class="work">
        <Cover title={entry.work.title} url={entry.work.cover_url} size="lg" />
        <div>
          <h1>{entry.work.title}</h1>
          <p class="meta">
            {[
              entry.work.year ? String(entry.work.year) : '',
              (entry.work.creators ?? []).map((creator) => creator.name).join(', '),
              mediumText(entry.work.medium),
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>
          {#if entry.work.verified && provider}
            <span class="tag"><Check size={13} aria-hidden="true" />{provider}</span>
          {:else if provider}
            <span class="tag unverified">{provider} · unverified</span>
          {/if}
        </div>
      </section>

      <section class="sec">
        <p class="stamp">
          {entry.finished_on ? dateText(entry.finished_on) : 'No date'}{entry.format
            ? ` · ${entry.format}`
            : ''}
        </p>
        <div class="ratingrow">
          <Rating quarters={entry.rating_q} readonly favourite={entry.favourite} showFavourite />
        </div>
        {#if entry.body}
          <p class="words">{entry.body}</p>
        {/if}
        {#if entry.verdict}
          <p class="verdict">“{entry.verdict}”</p>
        {/if}

        {#if groupChips.length > 0}
          <div class="desc">
            {#each groupChips as group (group.category)}
              <div class="dg">
                <span class="dl">{group.label}</span>
                <span class="chips">
                  {#each group.values as value (value)}
                    <span class="chip on">{value}</span>
                  {/each}
                </span>
              </div>
            {/each}
          </div>
        {/if}

        {#if questions.length > 0}
          <ul class="qa">
            {#each questions as answer (answer.prompt)}
              <li>
                <span class="qq">{answer.prompt}</span><span class="aa"
                  >{answer.values.join(', ')}</span
                >
              </li>
            {/each}
          </ul>
        {/if}

        {#if warnings.length > 0}
          <p class="cw"><b>Content warnings</b> {warnings.join(', ')}</p>
        {/if}

        {#if tags.length > 0}
          <div class="tags">
            {#each tags as tag (tag.name)}
              <Pill>{tag.name}</Pill>
            {/each}
          </div>
        {/if}

        {#if confirming}
          <div class="confirmbox">
            <p>
              <b>Delete this entry?</b> It moves to Deleted and disappears from your library. Nothing
              is erased, and you can restore it any time.
            </p>
            <div class="btnrow">
              <Button variant="primary" grow onclick={() => (confirming = false)}>Keep it</Button>
              <Button variant="danger" disabled={deleting} onclick={() => void remove()}>
                {deleting ? 'Deleting' : 'Delete'}
              </Button>
            </div>
          </div>
        {:else if !entry.deleted_at}
          <div class="btnrow">
            <Button grow onclick={edit}>Edit</Button>
            <Button grow onclick={again}>Log again</Button>
          </div>
        {/if}
        {#if revisions.length > 0}
          <button type="button" class="linkbtn versions" onclick={versions}>
            <History size={16} aria-hidden="true" />
            {revisions.length} earlier {revisions.length === 1 ? 'version' : 'versions'}
          </button>
        {/if}
      </section>

      {#if others.length > 0}
        <section class="sec">
          <p class="lbl"><span>Also on this work ({others.length})</span></p>
          {#each others as other (other.id)}
            <button
              type="button"
              class="tlrow"
              onclick={() => void goto(resolve('/entry/[id]', { id: other.id }))}
            >
              <span class="top">
                <span
                  >{shortDate(other.finished_on, thisYear)}{other.format
                    ? ` · ${other.format}`
                    : ''}</span
                >
                <span class="rt">{ratingText(other.rating_q)} ★</span>
              </span>
              {#if other.body}<span class="sn">{other.body}</span>{/if}
            </button>
          {/each}
        </section>
      {/if}

      <details class="details">
        <summary>Details<ChevronDown size={16} aria-hidden="true" /></summary>
        <dl class="kv">
          {#if entry.work.year}<dt>Year</dt>
            <dd>{entry.work.year}</dd>{/if}
          {#if entry.work.creators?.length}
            <dt>Creators</dt>
            <dd>
              {entry.work.creators.map((creator) => `${creator.name} (${creator.role})`).join(', ')}
            </dd>
          {/if}
          {#if entry.work.original_title}
            <dt>Original title</dt>
            <dd>{entry.work.original_title}</dd>
          {/if}
          {#if provider}<dt>Source</dt>
            <dd>{provider}</dd>{/if}
          {#if work?.sources?.length}
            <dt>Fetched</dt>
            <dd>
              {work.sources
                .map((source) => dateText(source.fetched_at?.slice(0, 10) ?? ''))
                .join(', ')}
            </dd>
          {/if}
          <dt>Saved</dt>
          <dd>{dateText(entry.created_at.slice(0, 10))}</dd>
          {#if revisions.length > 0}
            <dt>Last edited</dt>
            <dd>{dateText(entry.updated_at.slice(0, 10))}</dd>
          {/if}
        </dl>
      </details>
    {/if}
  </div>
</div>

<style>
  .work {
    display: flex;
    gap: 1rem;
    align-items: center;
    padding: 0.85rem 0 0.4rem;
  }
  .work h1 {
    margin: 0 0 0.2rem;
    font:
      400 1.5rem/1.2 'Fraunces',
      Georgia,
      serif;
    color: var(--text);
  }
  .meta {
    margin: 0 0 0.4rem;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font-size: 0.75rem;
  }
  .tag.unverified {
    background: var(--unverified-soft);
    color: var(--unverified);
  }
  .sec {
    padding: 0.85rem 0 0.35rem;
    border-top: 1px solid var(--line);
    margin-top: 0.85rem;
  }
  .stamp {
    margin: 0 0 0.6rem;
    font:
      500 0.6875rem 'Lexend',
      system-ui,
      sans-serif;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .ratingrow {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  .words {
    margin: 1rem 0 0;
    font-size: 1.0625rem;
    line-height: 1.65;
    overflow-wrap: anywhere;
  }
  .verdict {
    margin: 0.75rem 0 0;
    font:
      italic 400 1rem 'Fraunces',
      Georgia,
      serif;
    color: var(--text-2);
  }
  .desc {
    display: grid;
    gap: 0.55rem;
    margin-top: 1rem;
  }
  .dg {
    display: grid;
    grid-template-columns: 7.5rem 1fr;
    gap: 0.5rem;
    align-items: baseline;
  }
  .dl {
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }
  .chip.on {
    display: inline-flex;
    align-items: center;
    min-height: 28px;
    padding: 0 0.6rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font-size: 0.75rem;
  }
  .qa {
    margin: 1rem 0 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.4rem;
  }
  .qa li {
    display: grid;
    grid-template-columns: 7.5rem 1fr;
    gap: 0.5rem;
    font-size: 0.875rem;
  }
  .qq {
    color: var(--text-3);
  }
  .aa {
    color: var(--text);
  }
  .cw {
    margin: 1rem 0 0;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .cw b {
    color: var(--text);
    font-weight: 500;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-top: 0.85rem;
  }
  .versions {
    margin-top: 0.35rem;
  }
  .tlrow {
    display: grid;
    gap: 0.25rem;
    width: 100%;
    text-align: left;
    border: 0;
    border-bottom: 1px solid var(--line);
    background: none;
    padding: 0.7rem 0;
    min-height: 44px;
    color: inherit;
  }
  .tlrow .top {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .tlrow .rt {
    color: var(--accent);
    font-variant-numeric: tabular-nums;
  }
  .tlrow .sn {
    font-size: 0.875rem;
    color: var(--text-2);
    line-height: 1.45;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .details {
    margin-top: 1.25rem;
    border-top: 1px solid var(--line);
    padding-top: 0.6rem;
  }
  .details summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 44px;
    cursor: pointer;
    font-size: 0.875rem;
    color: var(--text-2);
    list-style: none;
  }
  .details summary::-webkit-details-marker {
    display: none;
  }
  .kv {
    display: grid;
    grid-template-columns: 7.5rem 1fr;
    gap: 0.4rem 0.5rem;
    margin: 0.4rem 0 0;
    font-size: 0.8125rem;
  }
  .kv dt {
    color: var(--text-3);
  }
  .kv dd {
    margin: 0;
    color: var(--text);
  }
  .deletednote {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    justify-content: space-between;
    flex-wrap: wrap;
    border: 1px solid var(--line);
    border-left: 3px solid var(--danger);
    background: var(--panel);
    border-radius: var(--r-md);
    padding: 0.75rem 0.9rem;
    margin-top: 0.5rem;
  }
  .deletednote p {
    margin: 0;
    font-size: 0.875rem;
    color: var(--text-2);
    max-width: 30ch;
  }
  .deletednote b {
    color: var(--text);
    font-weight: 500;
  }
</style>
