<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowUp } from '@lucide/svelte';
  import { ApiError, createClient, type Session, type SessionPage } from '../../lib/api/client';
  import Errbox from '../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { stashFirstMessage } from '../../lib/state/handoff';
  import { ageText } from '../../lib/state/reading';

  /**
   * The start of a library chat (mockup 04, frame 3). It asks a question, offers three ways in, and lists the chats she
   * has already had. Sending is what creates the session: a chat with no question in it would be an empty row in that
   * list forever, so the title is her question and the session is made when she asks it.
   */
  const client = createClient();

  const examples = [
    'which films did i give 4.5 or more?',
    'what did i read in march 2024?',
    'make hades 2 a 4.5',
  ];

  let recent = $state<Session[]>([]);
  let loading = $state(true);
  let problem = $state('');
  let asking = $state(false);
  let text = $state('');
  let restore = $state({ seq: 0, text: '' });
  let restored = $state(0);
  $effect(() => {
    if (restore.seq !== restored) {
      restored = restore.seq;
      text = restore.text;
    }
  });
  let composer = $state<HTMLTextAreaElement | null>(null);

  const now = new Date();

  async function loadRecent() {
    loading = true;
    problem = '';
    try {
      const sessions: SessionPage = await client.listSessions({ kind: 'general', limit: 5 });
      recent = sessions.items;
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your chats.';
    } finally {
      loading = false;
    }
  }

  void loadRecent();

  async function ask(question: string) {
    const trimmed = question.trim();
    if (trimmed === '' || asking) return;
    asking = true;
    problem = '';
    try {
      const session = await client.createSession({ kind: 'general', title: trimmed });
      // The question rides on the handoff, so the socket sends it once it has greeted: the same path a sentence typed
      // on Home takes, rather than a second way of starting a conversation.
      stashFirstMessage(trimmed);
      await goto(resolve('/chat/[id]', { id: session.id }));
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not start that chat.';
      asking = false;
    }
  }

  function send() {
    const question = text.trim();
    if (question === '' || asking) return;
    text = '';
    void ask(question);
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      send();
    }
  }
</script>

<svelte:head>
  <title>Ask your library · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Ask your library" back />

  <div class="scroll">
    <p class="intro">Ask about anything you have logged, or change an entry.</p>

    <div class="ex">
      {#each examples as example (example)}
        <button type="button" disabled={asking} onclick={() => void ask(example)}>{example}</button>
      {/each}
    </div>

    {#if problem}
      <Errbox message={problem} retryable onretry={() => void loadRecent()} />
    {:else if loading}
      <SkeletonRows rows={2} label="Reading your chats" />
    {:else if recent.length > 0}
      <div class="sect">
        <p class="lbl">Recent chats</p>
        <ul class="list">
          {#each recent as chat (chat.id)}
            <li class="row">
              <button
                type="button"
                class="ent"
                onclick={() => void goto(resolve('/chat/[id]', { id: chat.id }))}
              >
                <span class="q">{chat.title || 'A chat'}</span>
                <span class="age">{ageText(chat.updated_at, now)}</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>

  <form
    class="composer"
    onsubmit={(event) => {
      event.preventDefault();
      send();
    }}
  >
    <div class="box">
      <textarea
        bind:this={composer}
        bind:value={text}
        rows="1"
        placeholder="Ask about your library"
        aria-label="Ask about your library"
        {onkeydown}></textarea>
      <button type="submit" class="send" disabled={text.trim() === '' || asking} aria-label="Send">
        <ArrowUp size={18} aria-hidden="true" />
      </button>
    </div>
  </form>
</main>

<style>
  .intro {
    margin: 0.4rem 0 0;
    color: var(--text-2);
    font-size: 0.9375rem;
  }
  .ex {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 0.9rem;
  }
  .ex button {
    min-height: 44px;
    padding: 0.55rem 0.85rem;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--panel);
    color: var(--text);
    font: inherit;
    font-size: 0.9375rem;
    text-align: left;
  }
  .ex button:disabled {
    opacity: 0.6;
  }
  .sect {
    margin-top: 1.4rem;
  }
  .lbl {
    margin: 0 0 0.35rem;
    color: var(--text-3);
    font-size: 0.6875rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .row {
    border-bottom: 1px solid var(--line);
    list-style: none;
  }
  .row:last-child {
    border-bottom: 0;
  }
  .ent {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    width: 100%;
    min-height: 52px;
    padding: 0.6rem 0;
    border: 0;
    background: none;
    color: inherit;
    text-align: left;
  }
  .q {
    font-size: 0.9375rem;
    color: var(--text);
  }
  .age {
    flex: none;
    color: var(--text-3);
    font-size: 0.75rem;
  }
  .composer {
    position: sticky;
    bottom: 0;
    padding: 0.5rem 0 0.6rem;
    background: linear-gradient(to top, var(--bg) 70%, transparent);
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
