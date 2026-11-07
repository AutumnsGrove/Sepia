<script lang="ts">
  import type { Snippet } from 'svelte';

  /** A short label that stands on its own: a format, a year, a count. Not a control. */
  interface Props {
    children: Snippet;
  }
  let { children }: Props = $props();
</script>

<span class="pill"><span class="label">{@render children()}</span></span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    min-height: 28px;
    padding: 0 0.6rem;
    border-radius: 999px;
    background: var(--raised);
    color: var(--text-2);
    font-size: 0.75rem;
    line-height: 1;
    white-space: nowrap;
  }
  .label {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
  }
</style>
