<script lang="ts">
  import { Info } from '@lucide/svelte';
  import type { Snippet } from 'svelte';

  /** A quiet note in the flow of a screen. It never carries an error: that is Errbox. */
  interface Props {
    children: Snippet;
  }
  let { children }: Props = $props();
</script>

<div class="banner">
  <Info size={16} aria-hidden="true" />
  <span>{@render children()}</span>
</div>

<style>
  .banner {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    padding: 0.7rem 0.85rem;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .banner :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--text-3);
  }
</style>
