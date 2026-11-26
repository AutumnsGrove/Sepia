<script lang="ts">
  import { ArrowUp, Search, Square } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Errbox from './Errbox.svelte';
  import ListRow from './ListRow.svelte';
  import { errorText } from '../session/copy';
  import { entryMeta } from '../state/reading';
  import type { PendingMessage, ScreenHandlers } from '../session/screen';
  import type { ConnectionState } from '../socket/machine';
  import type { OpenInteraction, SessionState } from '../state/session';
  import { isWorking, timeline, type TimelineEntry } from '../state/timeline';

  /**
   * The library chat: her questions, the answers, the entries an answer is about, and a card for a change that is
   * waiting on her confirm. It owns no socket and no store, exactly like the logging screen, so the gallery can draw
   * every state from a fixture and a baseline cannot drift from the real screen.
   *
   * Mockup 04, frames 3 and 4 are the spec: an answer is a sentence with the entries under it, and a change is a card
   * with the diff, two buttons and one line saying the previous version is kept.
   */
  interface Props {
    session: SessionState;
    connection: ConnectionState;
    handlers: ScreenHandlers;
    pending?: PendingMessage[];
    problem?: string;
  }
  let { session, connection, handlers, pending = [], problem = '' }: Props = $props();

  const rows = $derived(timeline(session));
  const working = $derived(isWorking(session));
  const thisYear = new Date().getFullYear();
  // Closed and reconnecting are both "not right now": what she types is kept either way.
  const offline = $derived(connection === 'reconnecting' || connection === 'closed');

  let text = $state('');
  let composer = $state<HTMLTextAreaElement | null>(null);

  function send() {
    const trimmed = text.trim();
    if (trimmed === '' || working || session.status !== 'active') return;
    handlers.onsend(trimmed);
    text = '';
    if (composer) composer.style.height = '';
  }

  function onkeydown(event: KeyboardEvent) {
    // Enter sends, Shift+Enter is a new line: a phone keyboard's return key is the send key everywhere else here.
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      send();
    }
  }

  /** The change card's two answers, in the words the service checks. */
  function resolveEdit(interaction: OpenInteraction, choice: 'confirm' | 'cancel') {
    handlers.onanswer(interaction.id, { value: choice });
  }

  /** What an activity row says in a chat: the search it ran, in the words the mockup uses. */
  function activityText(row: { tool?: string; label: string }): string {
    if (row.tool === 'search_library') return 'Searched your library';
    if (row.tool === 'edit_entry') return 'Proposed a change';
    return row.label;
  }

  /** A stable key per row: the reducer's own order, with the kind so two rows at one order cannot collide. */
  function keyOf(row: TimelineEntry): string {
    return `${row.kind}-${row.order}`;
  }

  /** One row of the diff, as the card draws it: "Rating 4.25 4.5". */
  function changeRows(interaction: OpenInteraction) {
    const raw = interaction.payload.changes;
    if (!Array.isArray(raw)) return [];
    return raw.flatMap((entry) => {
      if (typeof entry !== 'object' || entry === null) return [];
      const change = entry as { label?: unknown; before?: unknown; after?: unknown };
      return [
        {
          label: typeof change.label === 'string' ? change.label : 'Change',
          before: typeof change.before === 'string' ? change.before : '',
          after: typeof change.after === 'string' ? change.after : '',
        },
      ];
    });
  }
</script>

<section class="chat" aria-label="Ask your library">
  {#if rows.length === 0 && !working}
    <p class="intro">Ask about anything you have logged, or change an entry.</p>
  {/if}

  {#each rows as row (keyOf(row))}
    {#if row.kind === 'block'}
      {#if row.block.kind === 'user'}
        <div class="you">
          <p class="who">You</p>
          <p class="said">{row.block.text}</p>
        </div>
      {:else}
        <p class="agent" class:streaming={row.block.streaming}>
          {row.block.text}
          {#if row.block.streaming}<span class="cursor" aria-hidden="true">▍</span>{/if}
        </p>
      {/if}
    {:else if row.kind === 'activity'}
      <!-- The mockup's line is what happened, in the past tense, not the count the tool returned: the count is the
           answer's business, and "1 match" beside an answer that already says "One book." says it twice. -->
      <div class="act">
        <Search size={14} aria-hidden="true" />
        <span>{activityText(row.row)}</span>
      </div>
    {:else if row.kind === 'notice'}
      <p class="notice">{row.row.text}</p>
    {:else if row.kind === 'results'}
      <ul class="res" aria-label="Entries this answer is about">
        {#each row.rows as result (result.entryID)}
          <ListRow
            title={result.title}
            meta={entryMeta(
              {
                completion: result.completion,
                finished_on: result.finishedOn,
                work: { medium: result.medium },
              } as never,
              thisYear,
            )}
            quarters={result.ratingQ > 0 ? result.ratingQ : null}
            medium={result.medium}
            coverUrl={result.coverUrl}
            onclick={handlers.onopenEntry
              ? () => handlers.onopenEntry?.(result.entryID)
              : undefined}
          />
        {/each}
      </ul>
    {:else if row.kind === 'interaction'}
      {#if row.interaction.kind === 'edit_entry'}
        {@const changes = changeRows(row.interaction)}
        <div class="confirmbox">
          <!-- The sentence that says nothing is saved is the answer's own text, above the card (mockup 04, frame 4).
               The card holds the change, the two buttons and the promise about the version, and says nothing twice. -->
          <p class="title">{String(row.interaction.payload.title ?? 'This entry')}</p>
          <ul class="chg">
            {#each changes as change (change.label)}
              <li>
                <span class="k">{change.label}</span>
                <span class="v">
                  {#if change.before}<span class="was">{change.before}</span>{/if}
                  {change.after || 'nothing'}
                </span>
              </li>
            {/each}
          </ul>
          <div class="foot">
            <Button variant="ghost" onclick={() => resolveEdit(row.interaction, 'cancel')}>
              Cancel
            </Button>
            <Button variant="primary" grow onclick={() => resolveEdit(row.interaction, 'confirm')}>
              Confirm change
            </Button>
          </div>
          <p class="note">The previous version is kept.</p>
        </div>
      {/if}
    {/if}
  {/each}

  {#each pending as message (message.id)}
    <div class="you pending">
      <p class="who">You</p>
      <p class="said">{message.text}</p>
      <span class="wait">{message.queued ? 'Will send when the answer is done' : 'Sending'}</span>
    </div>
  {/each}

  {#if problem}
    <Errbox message={problem} />
  {/if}

  {#if session.error}
    <Errbox
      message={errorText(session.error.code, session.error.message)}
      retryable
      onretry={handlers.onretry}
    />
  {/if}
</section>

{#if session.status === 'active'}
  <form
    class="composer"
    onsubmit={(event) => {
      event.preventDefault();
      send();
    }}
  >
    {#if offline}
      <p class="offline">You are offline. What you type is kept until you are back.</p>
    {/if}
    <div class="box">
      <textarea
        bind:this={composer}
        bind:value={text}
        rows="1"
        placeholder="Ask about your library"
        aria-label="Ask about your library"
        {onkeydown}></textarea>
      {#if working}
        <button type="button" class="send" aria-label="Stop" onclick={handlers.onstop}>
          <Square size={16} aria-hidden="true" />
        </button>
      {:else}
        <button type="submit" class="send" disabled={text.trim() === ''} aria-label="Send">
          <ArrowUp size={18} aria-hidden="true" />
        </button>
      {/if}
    </div>
  </form>
{/if}

<style>
  .chat {
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
    padding-bottom: 1.5rem;
  }
  .intro {
    margin: 0.4rem 0 0;
    color: var(--text-2);
    font-size: 0.9375rem;
  }
  .who {
    margin: 0 0 0.15rem;
    color: var(--text-3);
    font-size: 0.6875rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .said {
    margin: 0;
    color: var(--text);
    font-size: 1rem;
    line-height: 1.5;
  }
  .agent {
    margin: 0;
    color: var(--text);
    font-size: 1rem;
    line-height: 1.55;
  }
  /* The cursor is a block, so a streaming answer reads as unfinished rather than as a short one. */
  .cursor {
    display: inline-block;
    margin-left: 0.1rem;
    color: var(--accent);
    animation: blink 1.1s steps(2, start) infinite;
  }
  @keyframes blink {
    to {
      visibility: hidden;
    }
  }
  .act {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--text-3);
    font-size: 0.8125rem;
  }
  .notice {
    margin: 0;
    color: var(--text-2);
    font-size: 0.875rem;
  }
  .res {
    margin: 0;
    padding: 0;
  }
  .pending {
    opacity: 0.75;
  }
  .wait {
    display: block;
    margin-top: 0.2rem;
    color: var(--text-3);
    font-size: 0.75rem;
  }
  /* The change card. Quiet, because it is a card inside a conversation rather than a screen of its own. */
  .confirmbox {
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--panel);
    padding: 0.9rem 1rem 0.75rem;
  }
  .confirmbox .title {
    margin: 0;
    font:
      500 1.125rem 'Fraunces',
      Georgia,
      serif;
    color: var(--text);
  }
  .chg {
    margin: 0 0 0.85rem;
    padding: 0;
    list-style: none;
  }
  .chg li {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.45rem 0;
    border-top: 1px solid var(--line);
    font-size: 0.9375rem;
  }
  .chg .k {
    color: var(--text-2);
  }
  .chg .v {
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
  .chg .was {
    margin-right: 0.4rem;
    color: var(--text-3);
    text-decoration: line-through;
  }
  .foot {
    display: flex;
    gap: 0.6rem;
    align-items: center;
  }
  .confirmbox .note {
    margin: 0.6rem 0 0;
    color: var(--text-3);
    font-size: 0.75rem;
  }
  .composer {
    position: sticky;
    bottom: 0;
    padding: 0.5rem 0 0.6rem;
    background: linear-gradient(to top, var(--bg) 70%, transparent);
  }
  .offline {
    margin: 0 0 0.4rem;
    color: var(--text-2);
    font-size: 0.75rem;
  }
  .box {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
  }
  .box textarea {
    flex: 1;
    min-height: 44px;
    max-height: 140px;
    padding: 0.7rem 0.9rem;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--panel);
    color: var(--text);
    font: inherit;
    font-size: 0.9375rem;
    resize: none;
  }
  .box textarea:focus {
    outline: none;
    border-color: var(--accent);
  }
  .send {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    flex: none;
    border: 0;
    border-radius: 999px;
    background: var(--accent);
    color: var(--bg);
  }
  .send:disabled {
    background: var(--panel);
    color: var(--text-3);
  }
</style>
