<script lang="ts">
  import { CircleAlert, Check, Minus, RotateCcw } from '@lucide/svelte';
  import Button from '../../../lib/components/Button.svelte';
  import Errbox from '../../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../../lib/components/TopBar.svelte';
  import { ApiError, createClient, type BackupsInfo } from '../../../lib/api/client';
  import { deriveBackupsView } from '../../../lib/backup/view';

  /**
   * Backups. The nightly snapshot and the R2 mirror run on the potato, outside the container; this page reads the
   * receipts they leave and, for Back up now, writes one request file. Everything that decides what she sees is in
   * deriveBackupsView, so each state is a Vitest fixture.
   */
  const client = createClient();

  let info = $state<BackupsInfo | null>(null);
  let loading = $state(true);
  let problem = $state('');
  let asking = $state(false);
  let now = $state(Date.now());

  const view = $derived(info ? deriveBackupsView(info, now) : null);

  async function load() {
    loading = true;
    problem = '';
    try {
      info = await client.getBackups();
      now = Date.now();
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read the backups.';
    } finally {
      loading = false;
    }
  }

  async function runNow() {
    asking = true;
    problem = '';
    try {
      await client.runBackup();
      info = await client.getBackups();
      now = Date.now();
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not ask for a backup.';
    } finally {
      asking = false;
    }
  }

  $effect(() => {
    void load();
  });

  // A request that this page made is answered by the host, not by the server, so the page looks again while one is
  // waiting. The receipts are the answer; nothing here guesses when it will finish.
  $effect(() => {
    if (!info?.pending) return;
    const timer = setInterval(() => void load(), 5000);
    return () => clearInterval(timer);
  });
</script>

<svelte:head>
  <title>Backups · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Backups and export" back>
    {#snippet actions()}
      <button
        type="button"
        class="iconbtn"
        aria-label="Read the backups again"
        disabled={loading}
        onclick={() => void load()}
      >
        <RotateCcw size={20} aria-hidden="true" />
      </button>
    {/snippet}
  </TopBar>

  <div class="scroll">
    {#if loading && !info}
      <SkeletonRows rows={4} label="Reading the backups" />
    {:else if problem && !info}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if view}
      <dl class="kv">
        <dt>Last run</dt>
        <dd>{view.lastRun}</dd>
        <dt>Snapshot</dt>
        <dd>{view.snapshotSize || 'None yet'}</dd>
        <dt>On R2</dt>
        <dd>
          <span class="tag" class:ok={view.mirror === 'ok'} class:warn={view.mirror === 'failed'}>
            {#if view.mirror === 'ok'}
              <Check size={16} aria-hidden="true" />
            {:else if view.mirror === 'failed'}
              <CircleAlert size={16} aria-hidden="true" />
            {:else}
              <Minus size={16} aria-hidden="true" />
            {/if}
            {view.mirrorText}
          </span>
        </dd>
        <dt>Helper</dt>
        <dd>{view.helperText}</dd>
      </dl>

      {#if view.problem}
        <div class="panel bad">
          <p>
            The last backup had a problem. Your reviews are on the potato and nothing is lost. The
            next run tries again.
          </p>
          <p class="detail">{view.problem}</p>
        </div>
      {/if}

      {#if view.helper === 'not_installed' || view.helper === 'stale'}
        <div class="panel">
          <p>
            {view.helper === 'not_installed'
              ? 'Backups are not set up on this server.'
              : 'The backup helper has gone quiet, so nothing is being backed up.'}
            Everything is still in the database on this machine.
          </p>
          <p class="detail mono">{view.helperCommand}</p>
          <p class="fine">Run that on the potato to see why. See docs/INSTALL.md for the steps.</p>
        </div>
      {/if}

      <div class="btnrow">
        <Button
          variant="primary"
          grow
          disabled={!view.canRun || asking}
          onclick={() => void runNow()}
        >
          {asking || view.pending ? 'Backing up' : 'Back up now'}
        </Button>
      </div>
      <p class="fine">
        Keeps seven nights and four weeks, on the potato and on R2. Nothing is ever deleted from
        your library, so a backup holds every copy.
      </p>

      <p class="lbl">Recent runs</p>
      {#if view.runs.length === 0}
        <p class="fine">No backup has run yet.</p>
      {:else}
        <ul class="runs">
          {#each view.runs as run (run.id)}
            <li class:failed={run.failed}>
              <span>{run.when}</span>
              <span class="right">
                {#if run.failed}
                  Failed
                {:else}
                  {run.size}
                {/if}
              </span>
            </li>
          {/each}
        </ul>
      {/if}

      {#if problem}
        <Errbox message={problem} retryable onretry={() => void load()} />
      {/if}
    {/if}
  </div>
</main>

<style>
  .kv {
    display: grid;
    grid-template-columns: 7rem 1fr;
    gap: 0.45rem 1rem;
    margin: 0.5rem 0 0;
    font-size: 0.875rem;
  }
  .kv dt {
    color: var(--text-3);
  }
  .kv dd {
    margin: 0;
    color: var(--text);
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-2);
  }
  .tag.ok {
    color: var(--accent);
  }
  .tag.warn {
    color: var(--unverified);
  }
  .panel {
    margin-top: 1.1rem;
    padding: 0.85rem 0.95rem;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--panel);
    font-size: 0.875rem;
    line-height: 1.5;
    color: var(--text-2);
  }
  .panel.bad {
    border-color: color-mix(in oklch, var(--unverified) 45%, var(--line));
  }
  .panel p {
    margin: 0 0 0.5rem;
  }
  .panel p:last-child {
    margin-bottom: 0;
  }
  .detail {
    color: var(--text);
    word-break: break-word;
  }
  .btnrow {
    display: flex;
    gap: 0.6rem;
    margin-top: 1.1rem;
  }
  .fine {
    margin: 0.75rem 0 0;
    font-size: 0.8125rem;
    color: var(--text-3);
    line-height: 1.5;
  }
  .lbl {
    margin: 1.6rem 0 0.2rem;
    font: 500 0.6875rem 'Lexend';
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .runs {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .runs li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    min-height: 52px;
    border-bottom: 1px solid var(--line);
    font-size: 0.9375rem;
  }
  .runs li:last-child {
    border-bottom: 0;
  }
  .runs .right {
    color: var(--text-2);
    font-size: 0.875rem;
  }
  .runs li.failed .right {
    color: var(--danger);
  }
</style>
