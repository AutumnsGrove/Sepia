<script lang="ts">
  /**
   * Category tabs with a count of what is already covered. The selected tab is marked by a rule and by
   * `aria-selected`; the count is part of the accessible name, because a badge a screen reader skips is a
   * number only sighted people get.
   *
   * There is no `aria-controls` here: a tab may be used where no single panel exists (the gallery shows the
   * tabs alone), and pointing at an id that is not in the document is an axe violation. The screen that does
   * have a panel labels it with `aria-labelledby="tab-{id}"` and gives it `id="panel-{id}"`.
   */
  interface Tab {
    id: string;
    label: string;
    count?: number;
  }
  interface Props {
    tabs: Tab[];
    selected: string;
    label: string;
    onselect?: (id: string) => void;
  }
  let { tabs, selected, label, onselect }: Props = $props();
</script>

<div class="tabs" role="tablist" aria-label={label}>
  {#each tabs as tab (tab.id)}
    <button
      type="button"
      class="tab"
      role="tab"
      id="tab-{tab.id}"
      aria-selected={tab.id === selected}
      tabindex={tab.id === selected ? 0 : -1}
      onclick={() => onselect?.(tab.id)}
    >
      {tab.label}
      {#if tab.count !== undefined && tab.count > 0}
        <span class="tc" aria-hidden="true">{tab.count}</span>
        <span class="sr">{tab.count} covered</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .tabs {
    display: flex;
    gap: 1.1rem;
    overflow-x: auto;
    border-bottom: 1px solid var(--line);
    scrollbar-width: none;
  }
  .tabs::-webkit-scrollbar {
    display: none;
  }
  .tab {
    flex: none;
    min-height: 48px;
    padding: 0 0.1rem;
    background: none;
    border: 0;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    font-size: 0.9375rem;
    color: var(--text-2);
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    transition:
      color 150ms var(--ease),
      border-color 150ms var(--ease);
  }
  .tab[aria-selected='true'] {
    color: var(--text);
    border-bottom-color: var(--accent);
  }
  .tc {
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 99px;
    background: var(--accent-soft);
    color: var(--text);
    font:
      500 0.6875rem/18px 'Lexend',
      system-ui,
      sans-serif;
    text-align: center;
  }
</style>
