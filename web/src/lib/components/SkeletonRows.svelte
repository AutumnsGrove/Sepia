<script lang="ts">
  /**
   * The loading state of a list: eight rows of the same height as a real one, so the page does not jump when the
   * answer arrives and nothing spins in the middle of the screen (docs/slices/S12-reading-ui.md, Decisions 1).
   *
   * It is announced once, quietly, because a silent skeleton is a page that looks broken to a screen reader.
   */
  interface Props {
    rows?: number;
    label?: string;
  }
  let { rows = 8, label = 'Loading' }: Props = $props();
</script>

<div class="loading" role="status" aria-live="polite">
  <span class="sr">{label}</span>
  {#each Array.from({ length: rows }, (_, index) => index) as index (index)}
    <div class="skel" aria-hidden="true">
      <i class="c"></i>
      <span class="b">
        <i class="tt"></i>
        <i class="mm"></i>
      </span>
    </div>
  {/each}
</div>
