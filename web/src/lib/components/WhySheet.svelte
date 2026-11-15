<script lang="ts">
  import { ChevronRight, X } from '@lucide/svelte';
  import type { Draft } from '../contract/events';
  import type { Preread } from '../state/session';
  import { uiCopy } from '../session/copy';
  import { costWords } from '../session/cost';

  /**
   * The "why" sheet: every check with its odds, the tools that ran in order, the sources behind a web field, and
   * the cost split into what was consumed and what a plan covered. It is how "why does this entry say that" gets
   * answered, so it is plain numbers rather than a summary.
   *
   * It is a modal bottom sheet, not a block at the foot of the card: opened from an activity row, the old inline
   * block grew a thousand pixels below the fold and looked like the button had done nothing. The dialog also traps
   * focus and closes on Escape, which the page's bottom edge cannot do.
   */
  interface Props {
    why?: Draft['why'];
    preread?: Preread | null;
    /** Set by the screen when a "why" on an activity row asks for the sheet. */
    open?: boolean;
    onclose?: () => void;
  }
  let { why, preread = null, open = false, onclose }: Props = $props();

  let dialog = $state<HTMLDialogElement | null>(null);

  $effect(() => {
    if (open && dialog && !dialog.open) dialog.showModal();
  });

  function hide() {
    dialog?.close();
  }

  /** The dialog's own close event: Escape, the close button and a backdrop tap all arrive here. */
  function closed() {
    onclose?.();
  }

  interface Row {
    name: string;
    option: string;
    probability: number;
    fired: boolean;
  }

  // The card's own sheet is the truth when it has checks; the session's pre-read fills in while a card is still
  // being built. Either way the numbers are the ones that were sent, never recomputed.
  const rows = $derived<Row[]>(checks(why, preread));
  const tools = $derived(why?.tools ?? []);
  const prereadCost = $derived(why?.preread_cost ?? null);
  const turnCost = $derived(why?.turn_cost ?? null);

  function checks(why: Draft['why'], preread: Preread | null): Row[] {
    if (why && why.checks && why.checks.length > 0) {
      return why.checks.map((check) => ({
        name: check.check,
        option: check.option,
        probability: check.probability,
        fired: check.fired === true,
      }));
    }
    if (preread === null) return [];
    const fired = new Set(preread.fired ?? []);
    const out: Row[] = [];
    for (const [name, value] of Object.entries(preread.odds ?? {})) {
      if (typeof value !== 'object' || value === null) continue;
      for (const [option, probability] of Object.entries(value as Record<string, unknown>)) {
        if (typeof probability !== 'number') continue;
        out.push({ name, option, probability, fired: fired.has(name) });
      }
    }
    return out;
  }

  const percent = (value: number) => `${Math.round(value * 100)}%`;
</script>

<button type="button" class="whytoggle" aria-haspopup="dialog" onclick={() => dialog?.showModal()}>
  {uiCopy.whyTitle}
  <ChevronRight size={14} aria-hidden="true" />
</button>

<dialog
  bind:this={dialog}
  class="sheet"
  aria-label={uiCopy.whyTitle}
  onclose={closed}
  onclick={(event) => {
    // A click on the backdrop targets the dialog itself; the sheet's content stops at .sheet-inner.
    if (event.target === dialog) hide();
  }}
>
  <div class="sheet-inner">
    <header>
      <h2>{uiCopy.whyTitle}</h2>
      <button type="button" class="close" aria-label="Close" onclick={hide}>
        <X size={20} aria-hidden="true" />
      </button>
    </header>
    <div class="body">
      {#if rows.length > 0}
        <div class="odds-list">
          {#each rows as row (row.name + row.option)}
            <div class="o">
              <span>{row.name}: {row.option}</span>
              <span class="bar">
                <i class:hit={row.fired} style="width: {Math.round(row.probability * 100)}%"></i>
              </span>
              <span class="p">{percent(row.probability)}</span>
            </div>
          {/each}
        </div>
      {/if}
      {#if tools.length > 0}
        <div class="tools">
          <p class="lbl">Tools</p>
          <ol>
            {#each tools as tool (tool.tool + tool.summary)}
              <li class:bad={tool.ok === false}>
                <span class="name">{tool.tool}</span>
                <span class="sum">{tool.summary}</span>
                {#if tool.sources && tool.sources.length > 0}
                  <span class="srcs">
                    {#each tool.sources as source (source.url + source.title)}
                      <span>{source.title || source.url}</span>
                    {/each}
                  </span>
                {/if}
              </li>
            {/each}
          </ol>
        </div>
      {/if}
      {#if prereadCost || turnCost}
        <div class="costs">
          <p class="lbl">Cost</p>
          {#if prereadCost}
            <p>
              <span class="k">Pre-read</span>
              {costWords({ total: prereadCost.total_usd, covered: prereadCost.covered_usd })
                .consumed},
              {costWords({ total: prereadCost.total_usd, covered: prereadCost.covered_usd })
                .allowance}
            </p>
          {/if}
          {#if turnCost}
            <p>
              <span class="k">This turn</span>
              {costWords({ total: turnCost.total_usd, covered: turnCost.covered_usd }).consumed},
              {costWords({ total: turnCost.total_usd, covered: turnCost.covered_usd }).allowance}
            </p>
          {/if}
          <p class="note">{uiCopy.whyAllowance}</p>
        </div>
      {/if}
      {#if rows.length === 0 && tools.length === 0 && !prereadCost && !turnCost}
        <p class="note">Nothing to show yet.</p>
      {/if}
    </div>
  </div>
</dialog>

<style>
  /* The trigger stays in the card and is a full 44px row: the old summary was a 44px details marker, which is fine,
   * but the activity rows' "why" was 26x18 and looked like it did nothing. */
  .whytoggle {
    width: 100%;
    min-height: 44px;
    margin-top: 1.1rem;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    background: none;
    border: 0;
    border-top: 1px solid var(--line);
    color: var(--text-2);
    font-size: 0.8125rem;
    text-align: left;
    cursor: pointer;
  }
  .whytoggle:hover,
  .whytoggle:focus-visible {
    color: var(--text);
  }
  .whytoggle :global(svg) {
    flex: none;
    color: var(--text-3);
  }
  .sheet {
    position: fixed;
    inset: auto 0 0 0;
    margin: 0 auto;
    width: 100%;
    max-width: 44rem;
    max-height: 82dvh;
    padding: 0;
    border: 0;
    border-radius: var(--r-lg) var(--r-lg) 0 0;
    background: var(--panel);
    color: var(--text);
    overflow-y: auto;
    overscroll-behavior: contain;
  }
  .sheet::backdrop {
    background: rgb(0 0 0 / 0.55);
  }
  /* The dialog itself scrolls, so the close button is inside the scroll region and the region has focusable
   * content. A second scroll child needed a tabindex, which was both a Svelte a11y warning and an axe finding. */
  .sheet-inner {
    display: block;
  }
  header {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.5rem 0 1rem;
    background: var(--panel);
  }
  h2 {
    margin: 0;
    font:
      500 1rem 'Lexend',
      system-ui,
      sans-serif;
  }
  .close {
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 999px;
    background: none;
    color: var(--text-2);
  }
  .close:hover,
  .close:focus-visible {
    background: var(--raised);
    color: var(--text);
  }
  .body {
    padding: 0 1rem 1.75rem;
  }
  .odds-list {
    display: grid;
    gap: 0.45rem;
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .o {
    display: grid;
    grid-template-columns: 7.5rem 1fr 3rem;
    gap: 0.6rem;
    align-items: center;
  }
  .o > span:first-child {
    overflow-wrap: anywhere;
  }
  .bar {
    height: 4px;
    border-radius: 99px;
    background: var(--raised);
    overflow: hidden;
  }
  .bar i {
    display: block;
    height: 100%;
    background: var(--text-3);
    border-radius: 99px;
  }
  .bar i.hit {
    background: var(--accent);
    opacity: 0.75;
  }
  .p {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--text-3);
  }
  .lbl {
    font: 500 0.6875rem 'Lexend';
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
    margin: 1rem 0 0.4rem;
  }
  ol {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.45rem;
    font-size: 0.8125rem;
  }
  li {
    display: grid;
    gap: 0.1rem;
  }
  .name {
    color: var(--text);
  }
  li.bad .name {
    color: var(--danger);
  }
  .sum {
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .srcs {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    color: var(--text-3);
    overflow-wrap: anywhere;
  }
  .costs p {
    margin: 0 0 0.35rem;
    font-size: 0.8125rem;
    color: var(--text-2);
    line-height: 1.5;
  }
  .costs .k {
    color: var(--text-3);
    margin-right: 0.4rem;
  }
  .note {
    margin: 0.5rem 0 0;
    font-size: 0.75rem;
    color: var(--text-3);
  }
</style>
