<script lang="ts">
  /**
   * A chip is a toggle you tap to say something about the work. Three looks, and the state is carried by
   * `aria-pressed` as well as the fill, because colour alone is not allowed to say anything (STANDARDS 5).
   *
   * - plain: a selected filter, read back by the group around it
   * - pick: a describable quality she taps on
   * - hint: a suggestion the agent inferred, dashed until she taps it
   */
  interface Props {
    pressed?: boolean;
    look?: 'plain' | 'pick' | 'hint';
    disabled?: boolean;
    onclick?: (event: MouseEvent) => void;
    children: import('svelte').Snippet;
  }
  let { pressed = false, look = 'plain', disabled = false, onclick, children }: Props = $props();
</script>

<button
  type="button"
  class="chip"
  class:on={pressed}
  class:pick={look === 'pick'}
  class:sug={look === 'hint'}
  aria-pressed={pressed}
  {disabled}
  {onclick}
>
  {@render children()}
</button>

<style>
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
    transition:
      background 150ms var(--ease),
      border-color 150ms var(--ease);
  }
  .chip.pick {
    min-height: 40px;
    padding: 0 0.9rem;
    font-size: 0.875rem;
    background: var(--bg);
    color: var(--text);
  }
  .chip.pick:hover {
    background: var(--raised);
  }
  .chip.sug {
    border: 1.5px dashed var(--text-3);
    color: var(--text);
  }
  .chip.on {
    background: var(--accent-soft);
    border-color: transparent;
    color: var(--text);
  }
  .chip.pick.on {
    border-color: var(--accent);
  }
  .chip:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
</style>
