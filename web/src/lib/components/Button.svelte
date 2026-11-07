<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * The one button. `primary` is the single green action on a screen; `ghost` is a quiet one; `danger` never
   * appears without a confirmation next to it. A disabled primary keeps its label, because the reason it is
   * disabled belongs beside it.
   */
  interface Props {
    variant?: 'plain' | 'primary' | 'ghost' | 'danger';
    size?: 'md' | 'sm';
    grow?: boolean;
    disabled?: boolean;
    type?: 'button' | 'submit';
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  }

  let {
    variant = 'plain',
    size = 'md',
    grow = false,
    disabled = false,
    type = 'button',
    onclick,
    children,
  }: Props = $props();
</script>

<button
  {type}
  class="btn"
  class:primary={variant === 'primary'}
  class:ghost={variant === 'ghost'}
  class:danger={variant === 'danger'}
  class:grow
  class:sm={size === 'sm'}
  {disabled}
  {onclick}
>
  {@render children()}
</button>

<style>
  .btn {
    min-height: 48px;
    padding: 0 1.2rem;
    border-radius: var(--r-md);
    border: 1px solid var(--line);
    background: var(--panel);
    font:
      500 0.9375rem 'Lexend',
      system-ui,
      sans-serif;
    transition:
      background 150ms var(--ease),
      border-color 150ms var(--ease);
  }
  .btn:hover {
    background: var(--raised);
  }
  .btn.primary {
    background: var(--accent);
    color: var(--accent-ink);
    border-color: transparent;
  }
  .btn.primary:hover {
    background: color-mix(in oklch, var(--accent) 88%, white);
  }
  .btn.ghost {
    background: none;
    border-color: transparent;
    color: var(--text-2);
  }
  .btn.ghost:hover {
    background: var(--panel);
    color: var(--text);
  }
  .btn.grow {
    flex: 1;
  }
  .btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  .btn.sm {
    min-height: 40px;
    padding: 0 0.9rem;
    font-size: 0.8125rem;
    border-radius: var(--r-sm);
  }
  .btn.danger {
    background: none;
    border-color: var(--danger);
    color: var(--danger);
  }
  .btn.danger:hover {
    background: color-mix(in oklch, var(--danger) 18%, var(--bg));
  }
</style>
