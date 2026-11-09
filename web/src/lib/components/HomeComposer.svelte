<script lang="ts">
  import { Paperclip, Send } from '@lucide/svelte';
  import { messageProblem, MAX_MESSAGE } from '../session/composer';
  import { uiCopy } from '../session/copy';

  /**
   * Home's composer. It is the front door: what she types here starts a review, and the send button is off until there
   * is something to send. The attach button is disabled with its reason, because screenshots arrive with S18 and a
   * control that does nothing is a lie.
   */
  interface Props {
    onsend: (text: string) => void;
    /** A sentence tapped in the first-run examples lands here, not in the sent stream. */
    value?: string;
    placeholder?: string;
    busy?: boolean;
  }
  let {
    onsend,
    value = $bindable(''),
    placeholder = 'Say what you thought',
    busy = false,
  }: Props = $props();

  let field = $state<HTMLTextAreaElement | null>(null);
  const problem = $derived(messageProblem(value));
  const sendable = $derived(problem === '' && !busy);

  function grow() {
    if (!field) return;
    field.style.height = 'auto';
    field.style.height = `${Math.min(field.scrollHeight, 120)}px`;
  }

  function submit() {
    if (!sendable) return;
    onsend(value);
    value = '';
    if (field) field.style.height = 'auto';
  }

  function keydown(event: KeyboardEvent) {
    // Enter sends and Shift+Enter is a new line, which is what a phone keyboard's return key should do.
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  }
</script>

<form
  class="composer"
  onsubmit={(event) => {
    event.preventDefault();
    submit();
  }}
>
  {#if problem === 'too_long'}
    <p class="problem" role="status">
      Keep it under {MAX_MESSAGE.toLocaleString()} characters. That is longer than any review.
    </p>
  {/if}
  <div class="box">
    <button type="button" class="attach" aria-label={uiCopy.attachDisabled} disabled>
      <Paperclip size={22} aria-hidden="true" />
    </button>
    <textarea
      bind:this={field}
      bind:value
      rows="1"
      {placeholder}
      aria-label={placeholder}
      oninput={grow}
      onkeydown={keydown}></textarea>
    <button type="submit" class="send" aria-label="Send" disabled={!sendable}>
      <Send size={20} aria-hidden="true" />
    </button>
  </div>
</form>

<style>
  .composer {
    flex: none;
    padding: 0.5rem 0.75rem calc(0.9rem + env(safe-area-inset-bottom));
    background: var(--bg);
  }
  .problem {
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
  .attach {
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    border: 0;
    background: none;
    color: var(--text-3);
    border-radius: 999px;
    align-self: flex-end;
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
  }
  .send:disabled {
    background: var(--raised);
    color: var(--text-3);
  }
</style>
