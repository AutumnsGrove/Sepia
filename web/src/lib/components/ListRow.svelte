<script lang="ts">
  import { Star } from '@lucide/svelte';
  import Cover from './Cover.svelte';

  /**
   * One row in the library or the drafts list. `rating` shows the exact number beside a filled star, because a
   * star fill is a glance and the number is the fact. A draft row says what it is waiting for instead.
   */
  interface Props {
    title: string;
    meta?: string;
    coverUrl?: string | null;
    coverVariant?: 'default' | 'sand' | 'rust' | 'slate' | 'green' | 'plum' | 'teal' | 'ink';
    /** Quarters of a star, 0 to 20. Null or undefined means no rating yet. */
    quarters?: number | null;
    /** The sentence under a saved row: what she wrote. */
    note?: string;
    /** The quiet right-hand text on a draft: "2 h ago". */
    age?: string;
    /** What a draft is waiting for, shown with its own icon. */
    waiting?: string;
    onclick?: () => void;
  }
  let {
    title,
    meta,
    coverUrl = null,
    coverVariant = 'default',
    quarters = null,
    note,
    age,
    waiting,
    onclick,
  }: Props = $props();

  // The mockup shows 4.5 and 5, not 4.50: the number is read, not measured.
  const stars = $derived(quarters === null || quarters === undefined ? '' : String(quarters / 4));
</script>

<li class="row">
  <button type="button" class="ent" class:draft={waiting !== undefined} {onclick}>
    <Cover {title} url={coverUrl} variant={coverVariant} />
    <span class="body">
      <span class="tt">{title}</span>
      {#if meta}<span class="mm">{meta}</span>{/if}
      {#if note}<span class="sn">{note}</span>{/if}
      {#if waiting}<span class="st">{waiting}</span>{/if}
    </span>
    <span class="right">
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
  .age {
    font-size: 0.75rem;
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
