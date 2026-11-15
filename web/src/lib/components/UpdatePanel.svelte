<script lang="ts">
  import { RotateCcw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Errbox from './Errbox.svelte';
  import { sinceText, type UpdateView } from '../update/view';

  /**
   * The Updates panel, presentational: the page fetches and polls, the pure `deriveUpdateView` decides, and this file
   * draws one of the eight states from the mockup's page 05. Nothing here calls the server, so every state is a
   * fixture in the gallery and in the page's tests.
   */
  interface Props {
    view: UpdateView;
    /** The page's clock, so "last checked" is computed once per render rather than by a timer here. */
    now: number;
    oncheck?: () => void;
    onupdate?: () => void;
    onrestart?: () => void;
    busy?: boolean;
  }
  let { view, now, oncheck, onupdate, onrestart, busy = false }: Props = $props();
</script>

<div class="upd">
  {#if view.kind === 'loading'}
    <p class="big">Updates</p>
    <p class="small">Reading the update helper...</p>
  {:else if view.kind === 'no_updater'}
    <p class="big">Updates are not working on this server.</p>
    <p>
      The update helper {view.helper === 'stale' ? 'has gone quiet' : 'is not installed'}.
      Everything else still works.
    </p>
    <p class="detail mono cmd">{view.command}</p>
    <p class="small">Run that on the potato to see why. docs/INSTALL.md has the install steps.</p>
  {:else if view.kind === 'checking'}
    <p class="big">Checking for updates.</p>
    <div class="prog" role="progressbar" aria-label="Checking for updates"><i></i></div>
    <p class="small">This takes a few seconds.</p>
  {:else if view.kind === 'up_to_date'}
    {#if view.checkError}
      <Errbox message={`Sepia could not check for updates. ${view.checkError}`} />
      <p class="small">Everything else still works.</p>
    {:else}
      <p class="big">Up to date.</p>
      <p class="small">{view.checkedAt ? `Last checked ${sinceText(view.checkedAt, now)}.` : ''}</p>
    {/if}
    <Button variant="plain" disabled={busy} onclick={oncheck}>Check for updates</Button>
    {#if onrestart}
      <div class="restart">
        <button type="button" class="linkbtn" disabled={busy} onclick={onrestart}>
          <RotateCcw size={18} aria-hidden="true" /> Restart Sepia
        </button>
        <p class="small">Restart only. No new version, no rebuild.</p>
      </div>
    {/if}
  {:else if view.kind === 'available'}
    <p class="big">A newer version is ready.</p>
    <p class="latest mono">{view.latest}</p>
    <div class="full">
      <Button variant="primary" grow disabled={busy || view.blocked} onclick={onupdate}>
        Update now
      </Button>
    </div>
    {#if view.blocked}
      <p class="small">Finish the reply first. An update would stop it.</p>
    {:else}
      <p class="small">
        Takes about a minute. Your drafts are saved, and this page reconnects by itself.
      </p>
    {/if}
  {:else if view.kind === 'updating'}
    <p class="big">Updating</p>
    <div class="prog" role="progressbar" aria-label="Updating"><i></i></div>
    <p>This takes a minute. Sepia will reload when it is done.</p>
    <p class="small">Installing {view.target}.</p>
    {#if view.slow}
      <p class="small">This is taking longer than expected. It is still working.</p>
    {/if}
    <div class="full">
      <Button variant="plain" grow disabled>Update now</Button>
    </div>
  {:else if view.kind === 'done'}
    <p class="big">Updated to {view.version}.</p>
    <p class="small">Reloading the page...</p>
  {:else if view.kind === 'failed'}
    <Errbox
      message={view.rolledBack
        ? 'The update did not work, and the previous version is running again.'
        : 'Sepia may not be running properly. Check the potato.'}
    />
    <p class="detail mono">{view.detail}</p>
    {#if view.retryable}
      <div class="full">
        <Button variant="primary" grow onclick={onupdate}>Try again</Button>
      </div>
      <p class="small">Nothing was lost. Your entries and drafts are untouched.</p>
    {:else}
      <p class="small">journalctl -u sepia-update.service on the potato has the full log.</p>
    {/if}
  {/if}
</div>

<style>
  .upd {
    margin-top: 1.4rem;
  }
  .full {
    display: flex;
  }
  .big {
    margin: 0 0 0.9rem;
    font:
      500 1.125rem/1.35 'Lexend',
      sans-serif;
    color: var(--text);
  }
  .upd p {
    margin: 0 0 0.9rem;
    font-size: 1rem;
    line-height: 1.5;
    color: var(--text);
  }
  .small {
    font-size: 0.8125rem;
    color: var(--text-3);
    line-height: 1.5;
    margin: 0.9rem 0 0;
  }
  .latest {
    margin: -0.4rem 0 1.1rem;
  }
  .mono {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 0.8125rem;
    letter-spacing: 0;
    overflow-wrap: anywhere;
  }
  .detail {
    margin: 0.9rem 0 1rem;
    padding: 0.7rem 0.85rem;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    color: var(--text-2);
    line-height: 1.55;
    overflow-wrap: anywhere;
  }
  /* The command wraps rather than scrolling: a scroll region with nothing focusable inside it is an axe
   * violation, and the one command here is short enough to wrap. */
  .detail.cmd {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .prog {
    height: 4px;
    border-radius: 99px;
    background: var(--raised);
    overflow: hidden;
    margin: 1.1rem 0 0.9rem;
  }
  .prog i {
    display: block;
    height: 100%;
    width: 38%;
    border-radius: 99px;
    background: var(--accent);
    animation: slide 1.6s var(--ease) infinite;
  }
  @keyframes slide {
    0% {
      transform: translateX(-100%);
    }
    100% {
      transform: translateX(320%);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .prog i {
      animation: none;
      width: 100%;
      opacity: 0.5;
    }
  }
  .restart {
    margin-top: 2rem;
    border-top: 1px solid var(--line);
    padding-top: 0.6rem;
  }
  .linkbtn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 44px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text);
    font: inherit;
    cursor: pointer;
  }
  .linkbtn:disabled {
    color: var(--text-3);
    cursor: default;
  }
</style>
