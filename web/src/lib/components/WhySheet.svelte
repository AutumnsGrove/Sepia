<script lang="ts">
  import { ChevronRight } from '@lucide/svelte';
  import type { Draft } from '../contract/events';
  import type { Preread } from '../state/session';
  import { uiCopy } from '../session/copy';
  import { costWords } from '../session/cost';

  /**
   * The "why" sheet: every check with its odds, the tools that ran in order, the sources behind a web field, and
   * the cost split into what was consumed and what a plan covered. It is how "why does this entry say that" gets
   * answered, so it is plain numbers rather than a summary.
   */
  interface Props {
    why?: Draft['why'];
    preread?: Preread | null;
    /** Set by the screen when a "why" on an activity row asks for the sheet. A person can also just tap it. */
    open?: boolean;
  }
  let { why, preread = null, open = false }: Props = $props();
  let opened = $state(false);
  $effect(() => {
    if (open) opened = true;
  });

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

<details class="why" bind:open={opened}>
  <summary>
    {uiCopy.whyTitle}
    <ChevronRight size={14} aria-hidden="true" />
  </summary>
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
          {costWords({ total: prereadCost.total_usd, covered: prereadCost.covered_usd }).consumed},
          {costWords({ total: prereadCost.total_usd, covered: prereadCost.covered_usd }).allowance}
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
</details>

<style>
  .why {
    margin-top: 1.1rem;
    border-top: 1px solid var(--line);
    padding-top: 0.4rem;
  }
  summary {
    list-style: none;
    cursor: pointer;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary :global(svg) {
    transition: transform 150ms var(--ease);
  }
  .why[open] summary :global(svg) {
    transform: rotate(90deg);
  }
  .odds-list {
    display: grid;
    gap: 0.5rem;
    font-size: 0.8125rem;
    color: var(--text-2);
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
    height: 6px;
    border-radius: 99px;
    background: var(--bg);
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
