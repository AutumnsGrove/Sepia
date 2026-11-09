<script lang="ts">
  import { ChevronLeft } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { Snippet } from 'svelte';

  /**
   * The reading screens' top bar: a title with a way back, or the wordmark on Home with the three places to go. The
   * screen passes its own controls, so this file never has to know which screen it is on.
   */
  interface Props {
    /** The title. `wordmark` picks the serif treatment Home uses instead of a screen title. */
    title: string;
    wordmark?: boolean;
    /** Show the back button. */
    back?: boolean;
    /** The controls on the right. */
    actions?: Snippet;
  }
  let { title, wordmark = false, back = false, actions }: Props = $props();
</script>

<header class="topbar">
  {#if back}
    <button
      type="button"
      class="iconbtn"
      aria-label="Back"
      onclick={() => {
        if (window.history.length > 1) window.history.back();
        else void goto(resolve('/'));
      }}
    >
      <ChevronLeft size={22} aria-hidden="true" />
    </button>
  {/if}
  <!-- One heading per screen: the wordmark on Home, the screen's name everywhere else. -->
  <h1 class={wordmark ? 'wordmark' : 't'}>{title}</h1>
  {@render actions?.()}
  {#if back}
    <span class="iconbtn" aria-hidden="true"></span>
  {/if}
</header>
