<script lang="ts">
  import { CircleAlert } from '@lucide/svelte';
  import type { Snippet } from 'svelte';

  /**
   * An error with at least one way forward. The message is a plain sentence from the contract's vocabulary;
   * the actions are things she can actually do, never "dismiss" on its own.
   */
  interface Props {
    message: string;
    retryable?: boolean;
    onretry?: () => void;
    actions?: Snippet;
  }
  let { message, retryable = false, onretry, actions }: Props = $props();
</script>

<div class="errbox" role="alert">
  <CircleAlert size={18} aria-hidden="true" />
  <p>{message}</p>
  {#if retryable && onretry}
    <div class="acts">
      <button type="button" class="retry" onclick={onretry}>Try again</button>
    </div>
  {/if}
  {#if actions}
    <div class="acts">{@render actions()}</div>
  {/if}
</div>

<style>
  .errbox {
    display: flex;
    gap: 0.65rem;
    align-items: flex-start;
    flex-wrap: wrap;
    background: color-mix(in oklch, var(--danger) 14%, var(--panel));
    border-radius: var(--r-md);
    padding: 0.8rem 0.9rem;
  }
  .errbox :global(svg) {
    flex: none;
    margin-top: 1px;
    color: var(--danger);
  }
  p {
    margin: 0;
    flex: 1;
    min-width: 0;
    font-size: 0.9375rem;
    line-height: 1.5;
  }
  .acts {
    width: 100%;
    display: flex;
    gap: 0.5rem;
  }
  .retry {
    min-height: 44px;
    padding: 0 0.9rem;
    border-radius: var(--r-sm);
    border: 1px solid var(--line);
    background: none;
    color: var(--text);
    font-size: 0.8125rem;
  }
  .retry:hover {
    background: var(--raised);
  }
</style>
