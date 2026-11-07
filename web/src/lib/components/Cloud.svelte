<script lang="ts">
  import type { Snippet } from 'svelte';

  /** A wrapped run of chips. The wrapping is the whole component, so every cloud of tags looks the same. */
  interface Props {
    label?: string;
    children: Snippet;
  }
  let { label, children }: Props = $props();
</script>

<div class="cloud" role="group" aria-label={label}>
  {@render children()}
</div>

<style>
  .cloud {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }
</style>
