<script lang="ts">
  import { Check, CloudOff, Loader } from '@lucide/svelte';

  /**
   * The strip above the transcript that says what the connection is doing. The icon differs per state and the
   * words say it too, so a person who cannot tell amber from green still knows.
   */
  type State = 'ok' | 'offline' | 'working' | 'saved';
  interface Props {
    state: State;
    /** Overrides the default sentence for this state. */
    text?: string;
  }
  let { state, text }: Props = $props();

  const copy: Record<State, string> = {
    ok: "You're back and caught up.",
    offline: 'You are offline. What you type will be sent when you reconnect.',
    working: 'Working',
    saved: 'Saved to your library.',
  };
</script>

<div class="statusbar {state}" role="status" aria-live="polite">
  {#if state === 'ok'}
    <Check size={15} aria-hidden="true" />
  {:else if state === 'offline'}
    <CloudOff size={15} aria-hidden="true" />
  {:else if state === 'working'}
    <Loader class="spin" size={15} aria-hidden="true" />
  {:else}
    <Check size={15} aria-hidden="true" />
  {/if}
  <span>{text ?? copy[state]}</span>
</div>

<style>
  .statusbar {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    min-height: 36px;
    padding: 0 1rem;
    background: var(--panel);
    border-bottom: 1px solid var(--line);
    font-size: 0.8125rem;
    color: var(--text-2);
  }
  .statusbar :global(svg) {
    flex: none;
  }
  .statusbar.ok,
  .statusbar.saved {
    color: var(--text);
  }
  .statusbar.ok :global(svg),
  .statusbar.saved :global(svg) {
    color: var(--accent);
  }
  .statusbar.working :global(svg) {
    color: var(--text-3);
  }
  .spin {
    animation: turn 1.2s linear infinite;
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation: none;
    }
  }
</style>
