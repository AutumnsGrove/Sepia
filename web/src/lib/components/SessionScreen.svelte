<script lang="ts">
  import {
    ArrowLeft,
    Check,
    CircleAlert,
    Clock,
    Ellipsis,
    Info,
    Paperclip,
    Send,
    Square,
  } from '@lucide/svelte';
  import type { Entry } from '../api/client';
  import { copyFor } from '../contract/codes';
  import { composerState } from '../session/composer';
  import { errorText, problemText, uiCopy } from '../session/copy';
  import { saveState } from '../session/requirements';
  import {
    cardMode,
    sessionTitle,
    type PendingMessage,
    type ScreenHandlers,
  } from '../session/screen';
  import type { ConnectionState } from '../socket/machine';
  import type { SessionState } from '../state/session';
  import { liveActivity, timeline } from '../state/timeline';
  import Button from './Button.svelte';
  import EntryCard from './EntryCard.svelte';
  import Errbox from './Errbox.svelte';
  import InteractionCard from './InteractionCard.svelte';
  import Statusbar from './Statusbar.svelte';

  /**
   * The logging screen, state in and callbacks out. It owns no socket and no store: the route hands it the reducer's
   * state and the machine's connection, and the gallery hands it fixtures. That is what keeps a baseline and the
   * real screen from drifting apart.
   */
  interface Props {
    session: SessionState;
    connection: ConnectionState;
    handlers: ScreenHandlers;
    pending?: PendingMessage[];
    savedEntry?: Entry | null;
    versions?: number;
    /** Shown once after a reconnect, then cleared by the route. */
    caughtUp?: boolean;
    /** A refusal from the service, in its own words: a stale edit, a refused action, a lost message. */
    problem?: string;
  }
  let {
    session,
    connection,
    handlers,
    pending = [],
    savedEntry = null,
    versions = 0,
    caughtUp = false,
    problem = '',
  }: Props = $props();

  let text = $state('');
  let whyOpen = $state(false);
  let composer = $state<HTMLTextAreaElement | null>(null);

  const mode = $derived(cardMode(session, savedEntry));
  const closed = $derived(session.status !== 'active');
  const working = $derived(liveActivity(session) !== null);
  const rows = $derived(timeline(session));
  const save = $derived(saveState(session.draft));
  const unverifiedCount = $derived(
    (session.draft?.unverified ?? []).filter((field) => field.from_image !== true).length,
  );
  const need = $derived(
    save.reason ||
      (mode !== 'saved' && unverifiedCount > 0 ? 'Web fields stay unverified until accepted' : ''),
  );
  const composerView = $derived(composerState({ text, working, queue: session.queue, closed }));
  const offline = $derived(
    session.status === 'active' && (connection === 'reconnecting' || connection === 'closed'),
  );
  // A queued message and a closed session are states with their own words; only a refusal needs a line above the
  // composer, and the too-long message is the one she can still fix by editing.
  const composerProblem = $derived(
    composerView.problem === 'too_long' ? problemText('too_long') : '',
  );

  // A model failure is a warning, not an error (docs/PROTOCOL.md 7): her words are kept and the session stays
  // open, so the way on is the same Try again the error box offers.
  function retryableNotice(code: string): boolean {
    return code === 'model_unavailable' || code === 'turn_timeout' || code === 'turn_budget';
  }

  function submit() {
    if (!composerView.sendable) return;
    handlers.onsend(text);
    text = '';
    if (composer) composer.style.height = 'auto';
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  }

  function grow(event: Event) {
    const element = event.currentTarget as HTMLTextAreaElement;
    element.style.height = 'auto';
    element.style.height = `${Math.min(element.scrollHeight, 120)}px`;
  }
</script>

<div class="screen">
  {#if problem}
    <p class="sendproblem" role="alert">{problem}</p>
  {/if}
  <header class="topbar">
    <button type="button" class="iconbtn" aria-label="Back" onclick={handlers.onback}>
      <ArrowLeft size={22} aria-hidden="true" />
    </button>
    <div class="t">{sessionTitle(session)}</div>
    {#if handlers.onmore}
      <button type="button" class="iconbtn" aria-label="More" onclick={handlers.onmore}>
        <Ellipsis size={22} aria-hidden="true" />
      </button>
    {:else}
      <span class="iconbtn" aria-hidden="true"></span>
    {/if}
  </header>

  {#if offline}
    <Statusbar state="offline" text={uiCopy.offline} />
  {:else if caughtUp}
    <Statusbar state="ok" text={uiCopy.backOnline} />
  {/if}

  <div class="scroll">
    {#if session.status === 'saved'}
      <p class="saved-note"><Check size={16} aria-hidden="true" />{uiCopy.savedNote}</p>
    {/if}
    {#if mode === 'editing'}
      <div class="banner">
        <Info size={16} aria-hidden="true" />
        <span>{uiCopy.editingSaved}</span>
      </div>
    {/if}

    {#each rows as row (row.order)}
      {#if row.kind === 'block'}
        {#if row.block.kind === 'user'}
          <div class="you">
            <div class="who">You</div>
            <p>{row.block.text}</p>
          </div>
        {:else}
          <p class="agent" class:streaming={row.block.streaming}>
            {row.block.text}{#if row.block.streaming}<span class="cursor" aria-hidden="true">▍</span
              >{/if}
          </p>
        {/if}
      {:else if row.kind === 'activity'}
        <div class="act" class:live={row.row.live}>
          {#if row.row.live}
            <span class="dot" aria-hidden="true"></span>
          {:else if row.row.ok === false}
            <CircleAlert size={14} aria-hidden="true" />
          {:else}
            <Check size={14} aria-hidden="true" />
          {/if}
          <span>{row.row.label}</span>
          {#if session.draft && row.row.tool}
            <button type="button" class="why" onclick={() => (whyOpen = true)}>why</button>
          {/if}
        </div>
      {:else if row.kind === 'notice'}
        <div class="notice" class:warn={row.row.level === 'warn'}>
          <Info size={16} aria-hidden="true" />
          <span>{row.row.text || copyFor(row.row.code)}</span>
          {#if retryableNotice(row.row.code)}
            <button type="button" class="linkbtn" onclick={handlers.onretry}>Try again</button>
          {/if}
        </div>
      {:else if row.kind === 'interaction'}
        <InteractionCard
          kind={row.interaction.kind}
          payload={row.interaction.payload}
          onanswer={(value) => handlers.onanswer(row.interaction.id, value)}
        />
      {/if}
    {/each}

    {#each pending as message (message.id)}
      <div class="you pending">
        <div class="who">You</div>
        <p>{message.text}</p>
        <span class="pend">
          <Clock size={13} aria-hidden="true" />
          {message.queued ? uiCopy.queued : uiCopy.offlinePending}
          {#if message.queued}
            <button type="button" class="linkbtn" onclick={() => handlers.oncancel(message.id)}>
              Cancel
            </button>
          {/if}
        </span>
      </div>
    {/each}
    {#if pending.some((message) => message.queued)}
      <p class="hintbelow">{uiCopy.queuedHint}</p>
    {/if}

    {#if session.error}
      <div class="errorwrap">
        <Errbox
          message={errorText(session.error.code, session.error.message)}
          retryable={session.error.retryable}
          onretry={handlers.onretry}
        />
      </div>
    {/if}

    {#if session.draft}
      <EntryCard
        draft={session.draft}
        {mode}
        {savedEntry}
        {versions}
        preread={session.preread}
        {whyOpen}
        onedit={handlers.onedit}
        oninclude={handlers.oninclude}
        onaccept={handlers.onaccept}
        onacceptall={handlers.onacceptall}
        onversions={handlers.onversions}
      />
    {/if}
  </div>

  {#if session.status === 'active' && session.draft}
    <div class="confirm">
      <Button variant="ghost" onclick={handlers.ondiscard}>
        {mode === 'editing' ? uiCopy.cancel : uiCopy.discard}
      </Button>
      <span class="need">{need}</span>
      <Button variant="primary" disabled={!save.allowed} onclick={handlers.onconfirm}>
        {mode === 'editing' ? uiCopy.saveChanges : uiCopy.save}
      </Button>
    </div>
  {:else if session.status === 'saved'}
    <div class="confirm">
      {#if handlers.oneditSaved}
        <Button grow onclick={handlers.oneditSaved}>{uiCopy.edit}</Button>
      {/if}
      {#if handlers.onlogAnother}
        <Button grow variant="primary" onclick={handlers.onlogAnother}>{uiCopy.logAnother}</Button>
      {/if}
    </div>
  {/if}

  {#if session.status === 'active'}
    <form
      class="composer"
      onsubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      {#if composerProblem}
        <p class="composerproblem" role="status">{composerProblem}</p>
      {/if}
      <div class="box">
        <button type="button" class="attach" aria-label={uiCopy.attachDisabled} disabled>
          <Paperclip size={22} aria-hidden="true" />
        </button>
        <textarea
          bind:this={composer}
          bind:value={text}
          rows="1"
          placeholder={composerView.placeholder}
          aria-label="Say what you thought"
          disabled={composerView.disabled}
          oninput={grow}
          onkeydown={keydown}></textarea>
        {#if working}
          <button type="button" class="send stop" aria-label="Stop" onclick={handlers.onstop}>
            <Square size={16} aria-hidden="true" />
          </button>
        {:else}
          <button
            type="submit"
            class="send"
            aria-label="Send"
            disabled={!composerView.sendable || composerView.disabled}
          >
            <Send size={20} aria-hidden="true" />
          </button>
        {/if}
      </div>
    </form>
  {/if}
</div>

<style>
  .sendproblem {
    margin: 0;
    padding: 0.5rem 1rem;
    background: color-mix(in oklch, var(--danger) 14%, var(--panel));
    color: var(--text);
    font-size: 0.8125rem;
  }
  .screen {
    display: flex;
    flex-direction: column;
    min-height: 100dvh;
    max-width: 44rem;
    margin: 0 auto;
    background: var(--bg);
  }
  .topbar {
    height: 60px;
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.5rem 0 0.75rem;
  }
  .topbar .t {
    flex: 1;
    font: 500 1rem 'Lexend';
    text-align: center;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .iconbtn {
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    border: 0;
    background: none;
    border-radius: 999px;
    color: var(--text-2);
  }
  button.iconbtn:hover {
    background: var(--panel);
    color: var(--text);
  }
  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 0.25rem 1rem 1.25rem;
    scrollbar-width: none;
  }
  .scroll::-webkit-scrollbar {
    display: none;
  }
  .you {
    margin: 0.9rem 0 0;
  }
  .you .who {
    font: 500 0.6875rem 'Lexend';
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
    margin-bottom: 0.3rem;
  }
  .you p {
    margin: 0;
    font-size: 1.0625rem;
    line-height: 1.65;
    overflow-wrap: anywhere;
  }
  .you.pending {
    margin-top: 1.4rem;
  }
  .you.pending p {
    color: var(--text-2);
  }
  .pend {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.4rem;
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .act {
    margin: 0.9rem 0 0;
    font-size: 0.8125rem;
    line-height: 1.4;
    color: var(--text-3);
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .act.live {
    color: var(--text-2);
  }
  .act .why {
    background: none;
    border: 0;
    padding: 0;
    line-height: inherit;
    color: var(--text-2);
    text-decoration: underline;
    text-underline-offset: 3px;
    text-decoration-color: var(--line);
    font-size: 0.8125rem;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 99px;
    background: var(--accent);
    align-self: center;
    animation: pulse 1.4s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 1;
    }
  }
  .agent {
    margin: 0.9rem 0 0;
    font-size: 0.9375rem;
    color: var(--text-2);
    line-height: 1.55;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .cursor {
    color: var(--accent);
  }
  .notice {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    margin: 0.9rem 0 0;
    font-size: 0.8125rem;
    line-height: 1.5;
    color: var(--text-2);
  }
  .notice :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--text-3);
  }
  .notice.warn :global(svg) {
    color: var(--unverified);
  }
  .saved-note {
    margin: 1rem 0 0;
    font-size: 0.875rem;
    color: var(--text-2);
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .saved-note :global(svg) {
    color: var(--accent);
  }
  .banner {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    padding: 0.7rem 0.85rem;
    font-size: 0.8125rem;
    color: var(--text-2);
    margin-top: 1rem;
  }
  .banner :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--text-3);
  }
  .hintbelow {
    margin: 0;
    padding: 0.35rem 0 0;
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .errorwrap {
    margin-top: 1rem;
  }
  .confirm {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.7rem 0.75rem 0.9rem;
    background: var(--bg);
    border-top: 1px solid var(--line);
  }
  .confirm .need {
    flex: 1;
    font-size: 0.75rem;
    color: var(--text-3);
  }
  .composer {
    flex: none;
    padding: 0.5rem 0.75rem 0.9rem;
    background: var(--bg);
  }
  .composerproblem {
    margin: 0 0 0.4rem;
    font-size: 0.75rem;
    color: var(--danger);
  }
  .box {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 22px;
    padding: 0.35rem 0.35rem 0.35rem 1rem;
    transition: border-color 150ms var(--ease);
  }
  .box:focus-within {
    border-color: var(--accent);
  }
  textarea {
    flex: 1;
    border: 0;
    background: none;
    resize: none;
    outline: none;
    min-height: 44px;
    max-height: 120px;
    padding: 0.65rem 0;
    line-height: 1.4;
    font-size: 1rem;
  }
  textarea::placeholder {
    color: var(--text-3);
  }
  .send {
    width: 44px;
    height: 44px;
    border-radius: 999px;
    border: 0;
    background: var(--accent);
    color: var(--accent-ink);
    display: grid;
    place-items: center;
    flex: none;
    transition: background 150ms var(--ease);
  }
  .send[disabled] {
    background: var(--raised);
    color: var(--text-3);
    cursor: default;
  }
  .send.stop {
    background: var(--raised);
    color: var(--text);
    border: 1px solid var(--line);
  }
  .attach {
    width: 44px;
    height: 44px;
    border: 0;
    background: none;
    color: var(--text-3);
    display: grid;
    place-items: center;
    border-radius: 999px;
    flex: none;
  }
  .attach[disabled] {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .linkbtn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 32px;
    background: none;
    border: 0;
    padding: 0;
    font-size: 0.75rem;
    color: var(--text-2);
    text-decoration: underline;
    text-underline-offset: 3px;
    text-decoration-color: var(--line);
  }
  @media (prefers-reduced-motion: reduce) {
    .dot {
      animation: none;
    }
  }
</style>
