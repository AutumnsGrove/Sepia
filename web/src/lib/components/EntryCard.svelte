<script lang="ts">
  import {
    AlertTriangle,
    Check,
    ChevronRight,
    CircleDashed,
    Heart,
    History,
    Image,
    Link2,
  } from '@lucide/svelte';
  import type { Entry } from '../api/client';
  import type { Draft } from '../contract/events';
  import type { Preread } from '../state/session';
  import { uiCopy } from '../session/copy';
  import { whatWillChange } from '../session/diff';
  import { dateText, mediumText } from '../session/format';
  import Button from './Button.svelte';
  import Cover from './Cover.svelte';
  import Rating from './Rating.svelte';
  import WhySheet from './WhySheet.svelte';

  /**
   * The one card: an editable draft, a revision of a saved entry, or the saved entry itself. The blocks are in the
   * order docs/ENTRY_CARD.md 1 names them, and the saved card is the same card with editing off, because two
   * components would drift.
   *
   * Every change leaves as a `draft_edit` the session applies; the card never writes anything itself, and Save is
   * the screen's bar, not the card's.
   */
  interface Props {
    draft: Draft;
    /** `editing` is a revision draft: the same card, with the diff against the entry it revises. */
    mode?: 'draft' | 'editing' | 'saved';
    savedEntry?: Entry | null;
    versions?: number;
    preread?: Preread | null;
    whyOpen?: boolean;
    disabled?: boolean;
    onedit?: (set: Record<string, string | number | boolean>) => void;
    oninclude?: (segmentID: string) => void;
    onaccept?: (fieldID: string) => void;
    onacceptall?: () => void;
    onversions?: () => void;
  }
  let {
    draft,
    mode = 'draft',
    savedEntry = null,
    versions = 0,
    preread = null,
    whyOpen = false,
    disabled = false,
    onedit,
    oninclude,
    onaccept,
    onacceptall,
    onversions,
  }: Props = $props();

  const saved = $derived(mode === 'saved');
  const editable = $derived(!saved && !disabled);
  const changes = $derived(
    mode === 'editing' && savedEntry !== null ? whatWillChange(savedEntry, draft) : [],
  );

  let openSource = $state<string | null>(null);
  let openedSourceOnce = false;
  // The source panel opens with the card when the work came from the web: the mockup shows it beside the marker,
  // because the point is to compare what the web said with what she knows. Once only, so closing it stays closed.
  $effect(() => {
    if (!openedSourceOnce && draft.blocks.work === 'unverified') {
      openedSourceOnce = true;
      openSource = draft.unverified?.[0]?.id ?? 'work';
    }
  });
  let editingBody = $state(false);
  let bodyText = $state('');
  let editingFormat = $state(false);
  let editingDate = $state(false);

  const unverified = $derived(draft.unverified ?? []);
  const imageRead = $derived(unverified.filter((field) => field.from_image === true));
  const webFields = $derived(unverified.filter((field) => field.from_image !== true));
  const missing = $derived((draft.missing ?? []) as string[]);
  const needsRating = $derived(missing.includes('rating'));
  const tagAnswers = $derived(
    (draft.answers ?? []).filter(
      (answer) => answer.answer_type === 'tags' && (answer.values ?? []).length > 0,
    ),
  );
  const plainAnswers = $derived(
    (draft.answers ?? []).filter((answer) => answer.answer_type !== 'tags'),
  );

  const formats = [
    ['cinema', 'Cinema'],
    ['streaming', 'Streaming'],
    ['broadcast', 'TV broadcast'],
    ['audiobook', 'Audiobook'],
    ['hardback', 'Hardback'],
    ['paperback', 'Paperback'],
    ['ebook', 'Ebook'],
    ['physical', 'Physical'],
    ['live', 'Live'],
  ] as const;

  function formatLabel(value: string | undefined): string {
    return formats.find(([key]) => key === value)?.[1] ?? '';
  }

  function startBodyEdit() {
    if (!editable) return;
    bodyText = draft.body;
    editingBody = true;
  }

  function commitBody() {
    editingBody = false;
    if (bodyText !== draft.body) onedit?.({ body: bodyText });
  }

  function fieldID(field: NonNullable<Draft['unverified']>[number]): string {
    return field.id ?? field.field;
  }
</script>

<article class="card">
  <section class="work">
    <Cover title={draft.work.title} url={draft.work.cover_url ?? null} variant="sand" size="lg" />
    <div>
      <h3>{draft.work.title}</h3>
      <p class="meta">
        {#if draft.work.year}{draft.work.year} ·
        {/if}
        {#if draft.work.creators && draft.work.creators.length > 0}
          {draft.work.creators.map((creator) => creator.name).join(', ')} ·
        {/if}
        {mediumText(draft.work.medium)}
      </p>
      {#if draft.blocks.work === 'unverified'}
        <button
          type="button"
          class="markbtn u"
          aria-expanded={openSource !== null}
          onclick={() => (openSource = openSource === null ? (webFields[0]?.id ?? 'work') : null)}
        >
          <CircleDashed size={16} aria-hidden="true" />
          <span>{uiCopy.unverified}</span>
        </button>
      {:else if draft.blocks.work === 'verified' || draft.blocks.work === 'accepted' || draft.blocks.work === 'image_read'}
        <span class="tag v">
          <Check size={16} aria-hidden="true" />
          <span>{draft.work.primary_provider || 'Matched'}</span>
        </span>
      {/if}
    </div>
  </section>

  {#each webFields as field (fieldID(field))}
    {#if openSource === fieldID(field)}
      <div class="src">
        <span class="lnk">
          <Link2 size={14} aria-hidden="true" />
          {field.source_url || 'No source given'}
        </span>
        {#if field.snippet}<blockquote>{field.snippet}</blockquote>{/if}
        <div class="acts">
          {#if editable}
            <Button size="sm" onclick={() => onaccept?.(fieldID(field))}>Accept</Button>
          {/if}
          <Button size="sm" variant="ghost" onclick={() => (openSource = null)}>Close</Button>
        </div>
      </div>
    {/if}
  {/each}

  {#if editable && webFields.length > 0}
    <div class="acceptrow">
      <span class="t"
        ><CircleDashed size={16} aria-hidden="true" />{uiCopy.acceptAll(webFields.length)}</span
      >
      <Button size="sm" onclick={() => onacceptall?.()}>Accept all</Button>
    </div>
  {/if}
  {#if editable && imageRead.length > 0}
    <div class="acceptrow img">
      <span class="t"><Image size={16} aria-hidden="true" />{uiCopy.acceptImage}</span>
      <Button size="sm" onclick={() => onacceptall?.()}>Accept</Button>
    </div>
  {/if}

  <section>
    <p class="lbl">
      <span>
        Rating
        {#if needsRating}<span class="tag u needed"
            ><CircleDashed size={14} aria-hidden="true" />{uiCopy.needed}</span
          >{/if}
      </span>
      {#if editable}
        <button
          type="button"
          class="fav"
          aria-pressed={draft.rating.favourite}
          aria-label="Favourite"
          onclick={() => onedit?.({ favourite: !draft.rating.favourite })}
        >
          <Heart size={22} aria-hidden="true" />
        </button>
      {/if}
    </p>
    <Rating
      quarters={draft.rating.quarters}
      readonly={!editable}
      onchange={(quarters) => onedit?.({ rating: quarters })}
      label="Your rating"
    />
  </section>

  <section>
    <p class="lbl">Your words</p>
    {#if draft.verdict}
      <p class="verdict">{draft.verdict}</p>
    {/if}
    {#if editingBody}
      <textarea
        class="edit"
        rows="5"
        aria-label="Your words"
        bind:value={bodyText}
        onkeydown={(event) => {
          if (event.key === 'Escape') editingBody = false;
        }}></textarea>
      <div class="row">
        <Button size="sm" variant="primary" onclick={commitBody}>Done</Button>
        <Button size="sm" variant="ghost" onclick={() => (editingBody = false)}>Cancel</Button>
      </div>
    {:else if editable}
      <button type="button" class="wordsbtn" onclick={startBodyEdit}>
        <span class="words">{draft.body || 'Nothing yet. Tap to write your own.'}</span>
        <span class="sr">Edit your words</span>
      </button>
    {:else}
      <p class="words">{draft.body}</p>
    {/if}

    {#if draft.fidelity && draft.fidelity.status !== 'faithful'}
      <div class="warn">
        <AlertTriangle size={18} aria-hidden="true" />
        <span>
          {draft.fidelity.warning || 'Something you said may be missing. Check what was left out.'}
        </span>
      </div>
    {/if}

    {#if editable && draft.left_out && draft.left_out.length > 0}
      <details class="left" open={draft.fidelity?.status === 'drops_opinion'}>
        <summary>
          <ChevronRight size={14} aria-hidden="true" />
          {uiCopy.leftOut(draft.left_out.length)}
        </summary>
        <div class="segs">
          {#each draft.left_out as segment (segment.id)}
            <div class="seg">
              <span>{segment.text}</span>
              <Button size="sm" onclick={() => oninclude?.(segment.id)}>{uiCopy.include}</Button>
            </div>
          {/each}
        </div>
      </details>
    {/if}
  </section>

  <section>
    <p class="lbl">Facts</p>
    <div class="facts">
      {#if editingFormat}
        <label class="fact">
          <span class="k">Format</span>
          <select
            aria-label="Format"
            value={draft.format ?? ''}
            onchange={(event) => {
              editingFormat = false;
              const value = event.currentTarget.value;
              if (value !== (draft.format ?? '')) onedit?.({ format: value });
            }}
          >
            <option value="">Not set</option>
            {#each formats as [value, label] (value)}
              <option {value}>{label}</option>
            {/each}
          </select>
        </label>
      {:else}
        <button
          type="button"
          class="fact"
          disabled={!editable}
          onclick={() => (editingFormat = true)}
        >
          <span class="k">Format</span>
          <span class="val" class:empty={!draft.format}>
            {formatLabel(draft.format) || 'Add format'}
          </span>
        </button>
      {/if}
      {#if editingDate}
        <label class="fact">
          <span class="k">Finished</span>
          <input
            type="date"
            aria-label="Finished on"
            value={draft.finished_on ?? ''}
            onchange={(event) => {
              editingDate = false;
              const value = event.currentTarget.value;
              if (value !== (draft.finished_on ?? '')) onedit?.({ finished_on: value });
            }}
          />
        </label>
      {:else}
        <button
          type="button"
          class="fact"
          disabled={!editable}
          onclick={() => (editingDate = true)}
        >
          <span class="k">Finished</span>
          <span class="val" class:empty={!draft.finished_on}>{dateText(draft.finished_on)}</span>
        </button>
      {/if}
    </div>
  </section>

  {#if tagAnswers.length > 0 || plainAnswers.length > 0}
    <section>
      <p class="lbl">Answers</p>
      {#if tagAnswers.length > 0}
        <div class="desc">
          {#each tagAnswers as answer (answer.question_id)}
            <div class="dg">
              <span class="dl">{answer.category || answer.prompt || 'Answers'}</span>
              <span class="chips">
                {#each answer.values ?? [] as value (value)}
                  <span class="chip on">{value}</span>
                {/each}
              </span>
            </div>
          {/each}
        </div>
      {/if}
      {#if plainAnswers.length > 0}
        <ul class="qa">
          {#each plainAnswers as answer (answer.question_id)}
            <li>
              <span class="qq">{answer.prompt}</span>
              <span class="aa" class:skipped={answer.skipped || (answer.values ?? []).length === 0}>
                {answer.skipped || (answer.values ?? []).length === 0
                  ? 'Skipped'
                  : (answer.values ?? []).join(', ')}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}

  {#if draft.tags && draft.tags.length > 0}
    <section>
      <p class="lbl">Tags</p>
      <div class="chips">
        {#each draft.tags as tag (tag.name)}
          <span class="chip" class:sug={tag.source === 'genre'}>{tag.name}</span>
        {/each}
      </div>
    </section>
  {/if}

  {#if mode === 'editing' && changes.length > 0}
    <section>
      <p class="lbl">What will change</p>
      <ul class="diff">
        {#each changes as change (change.field)}
          <li>
            <span class="k">{change.label}</span>
            <span>
              {#if change.was}<span class="was">{change.was}</span>{/if}
              {change.now}
            </span>
          </li>
        {/each}
      </ul>
      {#if onversions && versions > 0}
        <button type="button" class="linkbtn" onclick={onversions}>
          <History size={16} aria-hidden="true" />
          {uiCopy.earlierVersions(versions)}
        </button>
      {/if}
    </section>
  {/if}

  {#if !saved}
    <WhySheet why={draft.why} {preread} open={whyOpen} />
  {/if}
</article>

<style>
  .card {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    padding: 1.1rem 1.1rem 1.25rem;
    margin-top: 1rem;
  }
  .card > * + * {
    margin-top: 1.4rem;
  }
  .work {
    display: grid;
    grid-template-columns: 84px 1fr;
    gap: 1rem;
    align-items: start;
  }
  h3 {
    margin: 0;
    font:
      500 1.625rem/1.12 'Fraunces',
      serif;
    font-variation-settings: 'opsz' 72;
    letter-spacing: -0.01em;
    overflow-wrap: anywhere;
  }
  .meta {
    margin: 0.4rem 0 0.5rem;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .lbl {
    font: 500 0.6875rem 'Lexend';
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
    margin: 0 0 0.55rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
  }
  .lbl > span {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }
  .needed {
    letter-spacing: 0;
    text-transform: none;
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.75rem;
    color: var(--text-2);
  }
  .tag.v {
    color: var(--accent);
  }
  .tag.u {
    color: var(--unverified);
  }
  .tag span {
    color: var(--text-2);
  }
  .markbtn {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 44px;
    margin: -10px 0;
    padding: 0 0.5rem 0 0;
    background: none;
    border: 0;
    font-size: 0.75rem;
    color: var(--unverified);
  }
  .markbtn span {
    color: var(--text-2);
    text-decoration: underline dotted;
    text-decoration-color: var(--text-3);
    text-underline-offset: 3px;
  }
  .src {
    margin-top: 0.6rem;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    padding: 0.8rem 0.9rem;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .src .lnk {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--text);
    word-break: break-all;
  }
  .src .lnk :global(svg) {
    flex: none;
    color: var(--text-3);
  }
  blockquote {
    margin: 0.5rem 0 0.75rem;
    padding: 0;
    font-style: italic;
    line-height: 1.55;
  }
  .src .acts {
    display: flex;
    gap: 0.5rem;
  }
  .acceptrow {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    background: var(--unverified-soft);
    border-radius: var(--r-md);
    padding: 0.55rem 0.55rem 0.55rem 0.9rem;
    font-size: 0.8125rem;
  }
  .acceptrow .t {
    flex: 1;
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }
  .acceptrow :global(svg) {
    flex: none;
    color: var(--unverified);
  }
  .acceptrow.img {
    background: var(--imageread-soft);
  }
  .acceptrow.img :global(svg) {
    color: var(--imageread);
  }
  .fav {
    width: 44px;
    min-height: 44px;
    margin: -12px -6px -12px 0;
    padding: 0;
    display: grid;
    place-items: center;
    background: none;
    border: 1px solid transparent;
    border-radius: 999px;
    color: var(--text-2);
  }
  .fav[aria-pressed='true'] {
    color: var(--accent);
    border-color: var(--accent);
  }
  .fav[aria-pressed='true'] :global(svg) {
    fill: currentColor;
  }
  .wordsbtn {
    display: block;
    width: 100%;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
  }
  .words {
    display: block;
    font-size: 1.125rem;
    line-height: 1.7;
    margin: 0;
    max-width: 62ch;
    overflow-wrap: anywhere;
  }
  .verdict {
    font: 400 italic 1rem/1.5 'Lexend';
    color: var(--text-2);
    margin: 0 0 0.75rem;
  }
  .edit {
    width: 100%;
    min-height: 10.5rem;
    background: var(--bg);
    border: 1px solid var(--accent);
    border-radius: var(--r-md);
    padding: 0.8rem 0.9rem;
    font:
      400 1.125rem/1.7 'Lexend',
      system-ui,
      sans-serif;
    resize: none;
    outline: none;
    color: var(--text);
  }
  .row {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.6rem;
  }
  .warn {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    background: var(--unverified-soft);
    border-radius: var(--r-md);
    padding: 0.75rem 0.85rem;
    font-size: 0.875rem;
    color: var(--text);
    margin-top: 0.75rem;
  }
  .warn :global(svg) {
    flex: none;
    margin-top: 1px;
    color: var(--unverified);
  }
  .left {
    margin-top: 0.85rem;
  }
  .left summary {
    list-style: none;
    cursor: pointer;
    font-size: 0.8125rem;
    color: var(--text-2);
    min-height: 40px;
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .left summary::-webkit-details-marker {
    display: none;
  }
  .left summary :global(svg) {
    transition: transform 150ms var(--ease);
  }
  .left[open] summary :global(svg) {
    transform: rotate(90deg);
  }
  .segs {
    display: grid;
    gap: 0.5rem;
  }
  .seg {
    display: flex;
    gap: 0.6rem;
    align-items: center;
    justify-content: space-between;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    padding: 0.65rem 0.8rem;
    font-size: 0.9375rem;
    color: var(--text-2);
  }
  .facts {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
  }
  .fact {
    display: block;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    padding: 0.6rem 0.8rem;
    min-height: 56px;
    text-align: left;
    color: inherit;
  }
  .fact:disabled {
    opacity: 1;
  }
  .fact .k {
    display: block;
    font-size: 0.6875rem;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .fact .val {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.9375rem;
    margin-top: 0.15rem;
  }
  .fact .val.empty {
    color: var(--text-3);
  }
  .fact select,
  .fact input {
    width: 100%;
    margin-top: 0.15rem;
    background: none;
    border: 0;
    color: var(--text);
    font-size: 0.9375rem;
    outline: none;
  }
  .desc {
    display: grid;
    gap: 0.95rem;
  }
  .dg {
    display: grid;
    gap: 0.4rem;
  }
  .dl {
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .chip {
    min-height: 36px;
    padding: 0 0.8rem;
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    border: 1px solid var(--line);
    font-size: 0.8125rem;
    color: var(--text-2);
    background: none;
  }
  .chip.on {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--text);
  }
  .chip.sug {
    border-style: dashed;
  }
  .qa {
    display: grid;
    gap: 0.55rem;
    margin: 0.9rem 0 0;
    padding: 0;
    list-style: none;
  }
  .qa li {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    font-size: 0.9375rem;
  }
  .qq {
    color: var(--text-2);
  }
  .aa {
    text-align: right;
  }
  .aa.skipped {
    color: var(--text-3);
  }
  .diff {
    display: grid;
    gap: 0.5rem;
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 0.875rem;
  }
  .diff li {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: baseline;
  }
  .diff .k {
    color: var(--text-2);
  }
  .diff .was {
    color: var(--text-3);
    text-decoration: line-through;
    margin-right: 0.5rem;
  }
  .linkbtn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 44px;
    background: none;
    border: 0;
    padding: 0;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
</style>
