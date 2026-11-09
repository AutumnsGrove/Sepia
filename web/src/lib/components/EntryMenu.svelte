<script lang="ts">
  import { History, MoreHorizontal, Pencil, Plus, Trash2 } from '@lucide/svelte';

  /**
   * The entry's menu. Delete lives here and only here: it is never offered in chat, and it is never the first item
   * under the thumb (docs/slices/S12-reading-ui.md). The panel closes on Escape, on a click outside, and after a
   * choice, so it can never be left open behind another screen.
   */
  interface Props {
    onedit: () => void;
    onagain: () => void;
    onversions: () => void;
    ondelete: () => void;
    /** The entry is already deleted: the menu then offers Restore instead of delete. */
    deleted?: boolean;
    onrestore?: () => void;
  }
  let { onedit, onagain, onversions, ondelete, deleted = false, onrestore }: Props = $props();

  let open = $state(false);
  let panel = $state<HTMLDivElement | null>(null);

  function toggle() {
    open = !open;
  }

  function close() {
    open = false;
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && open) {
      event.stopPropagation();
      close();
    }
  }

  // A click anywhere else closes the panel. The listener is only attached while it is open, so a closed menu costs
  // nothing on a page that is scrolled all the time.
  $effect(() => {
    if (!open) return;
    const away = (event: MouseEvent) => {
      if (panel && event.target instanceof Node && !panel.contains(event.target)) close();
    };
    document.addEventListener('click', away);
    return () => document.removeEventListener('click', away);
  });

  function choose(action: () => void) {
    return () => {
      close();
      action();
    };
  }
</script>

<div class="wrap" bind:this={panel} onkeydown={keydown} role="presentation">
  <button
    type="button"
    class="iconbtn"
    aria-label="More"
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={toggle}
  >
    <MoreHorizontal size={22} aria-hidden="true" />
  </button>
  {#if open}
    <div class="menu" role="menu">
      <button type="button" role="menuitem" onclick={choose(onedit)}>
        <Pencil size={18} aria-hidden="true" /> Edit
      </button>
      <button type="button" role="menuitem" onclick={choose(onagain)}>
        <Plus size={18} aria-hidden="true" /> Log again
      </button>
      <button type="button" role="menuitem" onclick={choose(onversions)}>
        <History size={18} aria-hidden="true" /> Earlier versions
      </button>
      {#if deleted}
        {#if onrestore}
          <button type="button" role="menuitem" onclick={choose(onrestore)}>
            <History size={18} aria-hidden="true" /> Restore
          </button>
        {/if}
      {:else}
        <button type="button" role="menuitem" class="danger" onclick={choose(ondelete)}>
          <Trash2 size={18} aria-hidden="true" /> Delete
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .wrap {
    position: relative;
  }
  .menu {
    position: absolute;
    top: 52px;
    right: 0;
    z-index: 10;
    min-width: 13rem;
    padding: 0.35rem;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--raised);
    box-shadow: 0 12px 28px oklch(0 0 0 / 0.45);
  }
  .menu button {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    min-height: 44px;
    padding: 0 0.7rem;
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text);
    font: 400 0.9375rem 'Lexend';
    text-align: left;
  }
  .menu button:hover,
  .menu button:focus-visible {
    background: var(--panel);
  }
  .menu .danger {
    color: var(--danger);
  }
</style>
