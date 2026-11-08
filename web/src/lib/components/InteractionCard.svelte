<script lang="ts">
  import { CircleAlert, Globe, RotateCcw, ScanText } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import Cloud from './Cloud.svelte';
  import Cover from './Cover.svelte';
  import Rating from './Rating.svelte';
  import Tabs from './Tabs.svelte';
  import { lookupSentence } from '../session/copy';

  /**
   * Every kind of question the agent can ask is one card, because a person should not have to learn a new
   * interface per question. Nothing is pre-selected, a guessed chip is dashed until she taps it, and the answer
   * leaves as one `interaction_response` the session engine applies.
   *
   * The payload is free-form in the contract (`{}`), so each kind's shape is read here with a local type. The
   * golden examples in api/examples/ are the shapes this is written against.
   */
  type Option = { value: string; label: string; hint?: string };
  type Candidate = {
    id: string;
    title: string;
    year?: number;
    creator?: string;
    medium?: string;
    thumb_url?: string;
  };
  type Tag = { value: string; label: string; tone?: string };
  type Category = { id: string; label: string; covered?: boolean; tags: Tag[]; hints?: string[] };
  type Question = { id: string; prompt: string; type: string; options?: string[]; hint?: string };

  interface Props {
    kind: string;
    payload: Record<string, unknown>;
    disabled?: boolean;
    onanswer: (value: unknown) => void;
  }
  let { kind, payload, disabled = false, onanswer }: Props = $props();

  const options = $derived((payload.options as Option[] | undefined) ?? []);
  const candidates = $derived((payload.candidates as Candidate[] | undefined) ?? []);
  const noneLabel = $derived((payload.none_label as string | undefined) ?? 'None of these');
  const categories = $derived((payload.categories as Category[] | undefined) ?? []);
  const questions = $derived((payload.questions as Question[] | undefined) ?? []);
  const previous = $derived((payload.previous as Record<string, unknown> | undefined) ?? {});
  const prompt = $derived((payload.prompt as string | undefined) ?? '');
  const reason = $derived((payload.reason as string | undefined) ?? '');
  const service = $derived((payload.service as string | undefined) ?? 'the service');
  const canAskPolaris = $derived(payload.can_ask_polaris === true);
  const gapField = $derived((payload.field as string | undefined) ?? '');

  // The tab she picked, or the first category until she picks one. Deriving it keeps the component honest when
  // a new interaction replaces the categories under it.
  let picked = $state<string | null>(null);
  // Arrays rather than Sets: they are copied on every change, so Svelte sees the update and there is no
  // mutable reactive instance to reason about.
  let chosen = $state<Record<string, string[]>>({});
  let answers = $state<Record<string, string>>({});
  let rating = $state(0);

  const tab = $derived(picked ?? categories[0]?.id ?? '');
  const activeCategory = $derived(
    categories.find((category) => category.id === tab) ?? categories[0],
  );
  const activeSet = $derived(chosen[tab] ?? []);

  const tabs = $derived(
    categories.map((category) => ({
      id: category.id,
      label: category.label,
      count: chosen[category.id]?.length ?? 0,
    })),
  );

  function isHint(category: Category, tag: Tag): boolean {
    return (category.hints ?? []).includes(tag.value);
  }

  function toggleTag(categoryID: string, value: string) {
    const current = chosen[categoryID] ?? [];
    const next = current.includes(value)
      ? current.filter((item) => item !== value)
      : [...current, value];
    chosen = { ...chosen, [categoryID]: next };
  }

  function allTags(): string[] {
    return Object.values(chosen).flat();
  }

  function submitDescribe() {
    onanswer({ tags: allTags() });
  }

  function submitQuestions() {
    onanswer({ answers });
  }

  function previousMeta(): string {
    const parts: string[] = [];
    if (typeof previous.finished_on === 'string') parts.push(`finished ${previous.finished_on}`);
    if (typeof previous.rating_q === 'number') parts.push(`${previous.rating_q / 4} stars`);
    return parts.join(' · ');
  }
</script>

<div class="ix">
  {#if kind === 'choose_medium'}
    <p class="q">Which one was it?</p>
    <p class="hintline">Your words could be more than one thing.</p>
    <div class="opts">
      {#each options as option (option.value)}
        <button
          type="button"
          class="opt"
          {disabled}
          onclick={() => onanswer({ value: option.value })}
        >
          <span class="l">{option.label}</span>
          {#if option.hint}<span class="s">{option.hint}</span>{/if}
        </button>
      {/each}
    </div>
  {:else if kind === 'choose_candidate'}
    <p class="q">Which one?</p>
    <p class="hintline">Tap the one you finished.</p>
    <div class="opts">
      {#each candidates as candidate (candidate.id)}
        <button
          type="button"
          class="opt cand"
          {disabled}
          onclick={() => onanswer({ value: candidate.id })}
        >
          <Cover title={candidate.title} url={candidate.thumb_url ?? null} size="sm" />
          <span class="body">
            <span class="tt">{candidate.title}</span>
            <span class="mm">
              {[candidate.creator, candidate.year].filter(Boolean).join(' · ')}
            </span>
          </span>
        </button>
      {/each}
      <button type="button" class="opt" {disabled} onclick={() => onanswer({ value: null })}>
        <span class="l">{noneLabel}</span>
      </button>
    </div>
  {:else if kind === 'describe'}
    <p class="q">Describe it</p>
    <p class="hintline">Pick any that fit, or none.</p>
    <Tabs {tabs} selected={tab} label="Categories" onselect={(id) => (picked = id)} />
    <div class="cloudwrap" role="tabpanel" id="panel-{tab}" aria-labelledby="tab-{tab}">
      {#if activeCategory}
        <p class="cloudnote">
          You mentioned this. Dashed ones are guesses from your words, and nothing is chosen until
          you tap.
        </p>
        <Cloud label={activeCategory.label}>
          {#each activeCategory.tags as tag (tag.value)}
            <Chip
              look={isHint(activeCategory, tag) ? 'hint' : 'pick'}
              pressed={activeSet.includes(tag.value)}
              {disabled}
              onclick={() => toggleTag(activeCategory.id, tag.value)}
            >
              {tag.label}
            </Chip>
          {/each}
        </Cloud>
      {/if}
    </div>
    <div class="formfoot">
      {#if allTags().length > 0}
        <Button variant="ghost" {disabled} onclick={() => (chosen = {})}>Clear</Button>
      {/if}
      <Button variant="primary" grow {disabled} onclick={submitDescribe}>Done</Button>
    </div>
  {:else if kind === 'question_form'}
    <div class="qf">
      {#each questions as question (question.id)}
        <div>
          <p class="lab">{question.prompt}</p>
          {#if question.type === 'choice'}
            <div class="pills">
              {#each question.options ?? [] as option (option)}
                <button
                  type="button"
                  class="pill"
                  aria-pressed={answers[question.id] === option}
                  class:hint={question.hint === option && !answers[question.id]}
                  {disabled}
                  onclick={() => (answers = { ...answers, [question.id]: option })}
                >
                  {option}
                </button>
              {/each}
            </div>
            {#if question.hint}
              <p class="hintnote">A guess from your words. Nothing is chosen until you tap.</p>
            {/if}
          {:else}
            <input
              class="field"
              type="text"
              placeholder="In your own words"
              value={answers[question.id] ?? ''}
              aria-label={question.prompt}
              {disabled}
              oninput={(event) =>
                (answers = { ...answers, [question.id]: event.currentTarget.value })}
            />
          {/if}
        </div>
      {/each}
    </div>
    <div class="formfoot">
      <Button variant="ghost" {disabled} onclick={() => onanswer({ answers: {} })}>Skip</Button>
      <Button variant="primary" grow {disabled} onclick={submitQuestions}>Done</Button>
    </div>
  {:else if kind === 'revisit_choice'}
    <p class="q">You have logged this before.</p>
    <p class="hintline">
      {previousMeta()}{#if typeof previous.verdict === 'string' && previous.verdict}— “{previous.verdict}”{/if}
    </p>
    <div class="opts">
      {#each options as option (option.value)}
        <button
          type="button"
          class="opt"
          {disabled}
          onclick={() => onanswer({ value: option.value })}
        >
          <span class="l">{option.label}</span>
        </button>
      {/each}
    </div>
  {:else if kind === 'fill_gap'}
    <p class="q">{prompt || 'One thing is missing.'}</p>
    {#if gapField === 'rating'}
      <p class="hintline">A quarter of a star at a time.</p>
      <Rating quarters={rating} onchange={(value) => (rating = value)} readonly={disabled} />
      <div class="formfoot">
        <Button variant="ghost" {disabled} onclick={() => onanswer({ value: null })}>Skip</Button>
        <Button variant="primary" grow {disabled} onclick={() => onanswer({ value: rating })}>
          {rating === 0 ? 'Leave it empty' : 'Set it'}
        </Button>
      </div>
    {:else}
      <input
        class="field"
        type="text"
        placeholder="In your own words"
        aria-label={prompt || 'The missing detail'}
        {disabled}
        oninput={(event) => (answers = { value: event.currentTarget.value })}
      />
      <div class="formfoot">
        <Button variant="ghost" {disabled} onclick={() => onanswer({ value: null })}>Skip</Button>
        <Button
          variant="primary"
          grow
          {disabled}
          onclick={() => onanswer({ value: answers.value ?? '' })}>Set it</Button
        >
      </div>
    {/if}
  {:else if kind === 'lookup_failed'}
    <p class="q"><CircleAlert size={18} aria-hidden="true" />{lookupSentence(service)}</p>
    <p class="hintline">{reason || 'It did not answer.'}</p>
    <div class="opts">
      <Button {disabled} onclick={() => onanswer({ value: 'retry' })}>
        <RotateCcw size={16} aria-hidden="true" /> Try again
      </Button>
      {#if canAskPolaris}
        <Button {disabled} onclick={() => onanswer({ value: 'ask_polaris' })}>
          <Globe size={16} aria-hidden="true" /> Ask Polaris
        </Button>
      {/if}
      <Button variant="ghost" {disabled} onclick={() => onanswer({ value: 'log_without' })}>
        <ScanText size={16} aria-hidden="true" /> Log without details
      </Button>
    </div>
  {:else}
    <p class="q">I need one thing.</p>
    <p class="hintline">This question is not one this version knows.</p>
  {/if}
</div>

<style>
  .ix {
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    padding: 1rem;
  }
  .q {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font:
      500 1rem 'Lexend',
      system-ui,
      sans-serif;
    margin: 0 0 0.15rem;
  }
  .q :global(svg) {
    color: var(--danger);
    flex: none;
  }
  .hintline {
    font-size: 0.8125rem;
    color: var(--text-3);
    margin: 0 0 0.85rem;
  }
  .opts {
    display: grid;
    gap: 0.5rem;
  }
  .opt {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    min-height: 52px;
    padding: 0.5rem 0.9rem;
    text-align: left;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    transition:
      background 150ms var(--ease),
      border-color 150ms var(--ease);
    color: inherit;
  }
  .opt:hover {
    background: var(--raised);
  }
  .opt:disabled {
    opacity: 0.45;
  }
  .opt.cand {
    display: grid;
    grid-template-columns: 44px 1fr;
    gap: 0.8rem;
    align-items: center;
  }
  .l {
    flex: 1;
    font-size: 0.9375rem;
  }
  .s {
    font-size: 0.8125rem;
    color: var(--text-3);
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
  }
  .mm {
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .cloudwrap {
    margin-top: 0.9rem;
  }
  .cloudnote {
    margin: 0 0 0.8rem;
    font-size: 0.8125rem;
    color: var(--text-3);
    line-height: 1.5;
  }
  .formfoot {
    display: flex;
    gap: 0.6rem;
    margin-top: 1.1rem;
  }
  .formfoot :global(.btn) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
  }
  .qf {
    display: grid;
    gap: 1.1rem;
  }
  .lab {
    font-size: 0.9375rem;
    margin: 0 0 0.5rem;
  }
  .pills {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }
  .pill {
    min-height: 44px;
    padding: 0 0.95rem;
    border-radius: 999px;
    background: var(--bg);
    border: 1px solid var(--line);
    font-size: 0.875rem;
    color: var(--text);
    transition:
      background 150ms var(--ease),
      border-color 150ms var(--ease);
  }
  .pill:hover {
    background: var(--raised);
  }
  .pill[aria-pressed='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .pill.hint {
    border: 1.5px dashed var(--text-3);
  }
  .hintnote {
    font-size: 0.75rem;
    color: var(--text-3);
    margin: 0.45rem 0 0;
  }
  .field {
    width: 100%;
    min-height: 48px;
    padding: 0.7rem 0.9rem;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    outline: none;
    color: var(--text);
  }
  .field:focus {
    border-color: var(--accent);
  }
  .field::placeholder {
    color: var(--text-3);
  }
</style>
