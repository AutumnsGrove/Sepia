<script lang="ts">
  import { Heart, Minus, Plus, Star } from '@lucide/svelte';

  /**
   * The rating control. Quarters of a star, 0 to 20, always with the exact number beside it: a fill is a glance,
   * the number is the fact. Drag across the stars, tap a star, use the buttons, or use the arrow keys. The
   * whole control is one slider for a screen reader, and every target is at least 44 px.
   */
  interface Props {
    /** Quarters of a star, 0 to 20. */
    quarters: number;
    onchange?: (quarters: number) => void;
    readonly?: boolean;
    label?: string;
    favourite?: boolean;
    onfavourite?: (favourite: boolean) => void;
    /** Shows the heart only when a favourite is something this screen can set. */
    showFavourite?: boolean;
  }
  let {
    quarters,
    onchange,
    readonly = false,
    label = 'Your rating',
    favourite = false,
    onfavourite,
    showFavourite = false,
  }: Props = $props();

  let starsBox = $state<HTMLDivElement | null>(null);
  let dragging = $state(false);

  const clamped = (value: number) => Math.max(0, Math.min(20, Math.round(value)));
  const words = $derived(quarters === 0 ? 'no rating' : `${quarters / 4} out of 5`);

  function set(value: number) {
    if (readonly) return;
    const next = clamped(value);
    if (next !== quarters) onchange?.(next);
  }

  function fromPointer(clientX: number) {
    if (!starsBox) return;
    const rect = starsBox.getBoundingClientRect();
    if (rect.width === 0) return;
    const ratio = (clientX - rect.left) / rect.width;
    // Each of the 20 steps is a quarter of a star; pointing at the left edge of a star is that star's first
    // quarter rather than nothing.
    set(Math.ceil(ratio * 20));
  }

  function pointerDown(event: PointerEvent) {
    if (readonly) return;
    dragging = true;
    (event.currentTarget as Element).setPointerCapture?.(event.pointerId);
    fromPointer(event.clientX);
    event.preventDefault();
  }

  function pointerMove(event: PointerEvent) {
    if (dragging) fromPointer(event.clientX);
  }

  function keydown(event: KeyboardEvent) {
    if (readonly) return;
    const step = event.shiftKey ? 4 : 1;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowUp':
        set(quarters + step);
        break;
      case 'ArrowLeft':
      case 'ArrowDown':
        set(quarters - step);
        break;
      case 'Home':
        set(0);
        break;
      case 'End':
        set(20);
        break;
      default:
        return;
    }
    event.preventDefault();
  }

  /** How much of star `index` is filled, as a percentage. */
  function fill(index: number): number {
    const within = quarters - index * 4;
    return (Math.max(0, Math.min(4, within)) / 4) * 100;
  }
</script>

<div class="rate">
  <div
    class="stars"
    role="slider"
    tabindex={readonly ? -1 : 0}
    aria-label={label}
    aria-valuemin={0}
    aria-valuemax={20}
    aria-valuenow={quarters}
    aria-valuetext={words}
    aria-readonly={readonly}
    bind:this={starsBox}
    onpointerdown={pointerDown}
    onpointermove={pointerMove}
    onpointerup={() => (dragging = false)}
    onpointercancel={() => (dragging = false)}
    onkeydown={keydown}
  >
    {#each [0, 1, 2, 3, 4] as index (index)}
      <span class="star">
        <Star class="base" size={30} />
        <span class="fillbox" style="width: {fill(index)}%">
          <Star size={30} />
        </span>
      </span>
    {/each}
  </div>

  <button
    type="button"
    class="step-btn"
    aria-label="A quarter star less"
    disabled={readonly || quarters === 0}
    onclick={() => set(quarters - 1)}><Minus size={20} aria-hidden="true" /></button
  >
  <span class="num" aria-hidden="true">{quarters / 4}</span>
  <button
    type="button"
    class="step-btn"
    aria-label="A quarter star more"
    disabled={readonly || quarters === 20}
    onclick={() => set(quarters + 1)}><Plus size={20} aria-hidden="true" /></button
  >

  {#if showFavourite}
    <button
      type="button"
      class="fav"
      aria-pressed={favourite}
      aria-label="Favourite"
      disabled={readonly}
      onclick={() => onfavourite?.(!favourite)}
    >
      <Heart size={16} aria-hidden="true" />
      <span>Favourite</span>
    </button>
  {/if}
</div>

<style>
  .rate {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .stars {
    position: relative;
    display: inline-flex;
    touch-action: none;
    user-select: none;
    padding: 6px 0;
    cursor: pointer;
  }
  .stars[aria-readonly='true'] {
    cursor: default;
  }
  .star {
    position: relative;
    width: 30px;
    height: 30px;
    flex: none;
  }
  .star :global(svg) {
    position: absolute;
    inset: 0;
    width: 30px;
    height: 30px;
  }
  .star :global(svg.base) {
    color: var(--text-3);
    opacity: 0.5;
  }
  .fillbox {
    position: absolute;
    inset: 0 auto 0 0;
    overflow: hidden;
    color: var(--accent);
  }
  .fillbox :global(svg) {
    fill: var(--accent);
    stroke: var(--accent);
  }
  .num {
    font:
      500 1.25rem 'Lexend',
      system-ui,
      sans-serif;
    font-variant-numeric: tabular-nums;
    min-width: 3ch;
  }
  .step-btn {
    width: 44px;
    height: 44px;
    border-radius: 999px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--text-2);
    display: grid;
    place-items: center;
  }
  .step-btn:hover {
    background: var(--raised);
    color: var(--text);
  }
  .step-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .fav {
    margin-left: auto;
    min-height: 44px;
    padding: 0 0.85rem;
    border-radius: 999px;
    background: none;
    border: 1px solid var(--line);
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .fav[aria-pressed='true'] {
    color: var(--accent);
    border-color: var(--accent);
  }
  .fav[aria-pressed='true'] :global(svg) {
    fill: var(--accent);
  }
  .fav:disabled {
    opacity: 0.4;
  }
</style>
