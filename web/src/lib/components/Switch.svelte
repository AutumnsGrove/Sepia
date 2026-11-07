<script lang="ts">
  /** A two-state switch with a real name. It is a button with `aria-checked`, not a checkbox in a label. */
  interface Props {
    checked: boolean;
    label: string;
    disabled?: boolean;
    onchange?: (checked: boolean) => void;
  }
  let { checked, label, disabled = false, onchange }: Props = $props();
</script>

<button
  type="button"
  class="switch"
  role="switch"
  aria-checked={checked}
  aria-label={label}
  {disabled}
  onclick={() => onchange?.(!checked)}
>
  <i aria-hidden="true"></i>
</button>

<style>
  .switch {
    position: relative;
    flex: none;
    width: 52px;
    height: 32px;
    border-radius: 99px;
    border: 1px solid var(--line);
    background: var(--bg);
    transition:
      background 150ms var(--ease),
      border-color 150ms var(--ease);
  }
  .switch i {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 24px;
    height: 24px;
    border-radius: 99px;
    background: var(--text-3);
    transition:
      transform 150ms var(--ease),
      background 150ms var(--ease);
  }
  .switch[aria-checked='true'] {
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .switch[aria-checked='true'] i {
    transform: translateX(20px);
    background: var(--accent);
  }
  .switch:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
</style>
