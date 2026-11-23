<script lang="ts">
  import { Heart, Star } from '@lucide/svelte';
  import Cover from './Cover.svelte';
  import type { SnippetSegment } from '../state/search';

  /**
   * One row in the library or the drafts list. `rating` shows the exact number beside a filled star, because a
   * star fill is a glance and the number is the fact. A draft row says what it is waiting for instead.
   */
  interface Props {
    title: string;
    meta?: string;
    coverUrl?: string | null;
    coverVariant?: 'default' | 'sand' | 'rust' | 'slate' | 'green' | 'plum' | 'teal' | 'ink';
    /** The work's medium, which colours the cover's stand-in when no variant is named. */
    medium?: string;
    /** Quarters of a star, 0 to 20. Null or undefined means no rating yet. */
    quarters?: number | null;
    /** A favourite carries a heart beside its rating, which is a second signal and never the only one. */
    favourite?: boolean;
    /** The sentence under a saved row: what she wrote. */
    note?: string;
    /**
     * The same sentence as runs, for a search result: the matched words are marked and everything else is escaped
     * like any other text. It is a fallback-free alternative to note, so one of the two is rendered.
     */
    noteSegments?: SnippetSegment[];
    /** The quiet right-hand text on a draft: "2 h ago". */
    age?: string;
    /** What a draft is waiting for, shown with its own icon. */
    waiting?: string;
    /** The shelf, when it is not an ordinary finished review: "didn't finish" or "wanted". */
    badge?: string;
    onclick?: () => void;
  }
  let {
    title,
    meta,
    coverUrl = null,
    coverVariant,
    medium = '',
    quarters = null,
    favourite = false,
    note,
    noteSegments,
    age,
    waiting,
    badge,
    onclick,
  }: Props = $props();

  // The mockup shows 4.5 and 5, not 4.50: the number is read, not measured.
  const stars = $derived(quarters === null || quarters === undefined ? '' : String(quarters / 4));
</script>

<li class="row">
  <button type="button" class="ent" class:draft={waiting !== undefined} {onclick}>
    <Cover {title} url={coverUrl} variant={coverVariant} {medium} />
    <span class="body">
      <span class="tt">{title}</span>
      {#if meta}<span class="mm">{meta}</span>{/if}
      {#if badge}<span class="shelf">{badge}</span>{/if}
      {#if noteSegments && noteSegments.length > 0}
        <span class="sn">
          {#each noteSegments as segment, index (index)}{#if segment.marked}<mark class="m"
                >{segment.text}</mark
              >{:else}{segment.text}{/if}{/each}
        </span>
      {:else if note}
        <span class="sn">{note}</span>
      {/if}
      {#if waiting}<span class="st">{waiting}</span>{/if}
    </span>
    <span class="right">
      {#if favourite}
        <span class="hrt" aria-label="A favourite"><Heart size={13} aria-hidden="true" /></span>
      {/if}
      {#if quarters !== null && quarters !== undefined}
        <span class="rt" aria-label="{stars} out of 5 stars">
          <Star size={14} aria-hidden="true" />
          <span aria-hidden="true">{stars}</span>
        </span>
      {/if}
      {#if age}<span class="age">{age}</span>{/if}
    </span>
  </button>
</li>

<style>
  .row {
    border-bottom: 1px solid var(--line);
    list-style: none;
  }
  .row:last-child {
    border-bottom: 0;
  }
  .ent {
    display: grid;
    grid-template-columns: 44px 1fr auto;
    gap: 0.85rem;
    align-items: center;
    padding: 0.75rem 0;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    min-height: 72px;
    color: inherit;
  }
  .ent.draft {
    grid-template-columns: 44px 1fr auto;
  }
  .body {
    display: grid;
    gap: 0.1rem;
    min-width: 0;
  }
  .tt {
    font:
      500 1.0625rem/1.25 'Fraunces',
      Georgia,
      serif;
    font-variation-settings: 'opsz' 24;
    color: var(--text);
  }
  .mm {
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  /* The shelf badge. Quiet on purpose: it is a fact about the row, not a warning about it. */
  .shelf {
    justify-self: start;
    margin-top: 0.15rem;
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    font-size: 0.6875rem;
    letter-spacing: 0.02em;
    color: var(--text-2);
    white-space: nowrap;
  }
  .sn {
    margin-top: 0.25rem;
    font-size: 0.875rem;
    color: var(--text-2);
    line-height: 1.45;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  /* The matched word. A soft wash plus an underline, so the highlight survives the greyscale of a screenshot and
     is not the only signal: the word is different from the ones around it either way. */
  .sn :global(mark.m) {
    background: var(--accent-soft);
    color: var(--text);
    border-radius: 3px;
    padding: 0 2px;
    box-shadow: inset 0 -1.5px 0 var(--accent);
  }
  .st {
    font-size: 0.8125rem;
    color: var(--text-2);
    margin-top: 0.2rem;
  }
  .right {
    display: grid;
    justify-items: end;
    gap: 0.2rem;
  }
  .rt {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font:
      500 0.9375rem 'Lexend',
      system-ui,
      sans-serif;
    font-variant-numeric: tabular-nums;
    color: var(--accent);
  }
  .rt :global(svg) {
    fill: var(--accent);
    stroke: var(--accent);
  }
  .hrt {
    display: inline-flex;
    color: var(--accent);
  }
  .hrt :global(svg) {
    fill: var(--accent);
    stroke: var(--accent);
  }
  .age {
    font-size: 0.75rem;
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
