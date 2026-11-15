<script lang="ts">
  import {
    ApiError,
    createClient,
    type UpdateAvailable,
    type UpdateStatus,
    type VersionInfo,
  } from '../../../lib/api/client';
  import Errbox from '../../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../../lib/components/SkeletonRows.svelte';
  import TopBar from '../../../lib/components/TopBar.svelte';
  import UpdatePanel from '../../../lib/components/UpdatePanel.svelte';
  import { deriveUpdateView } from '../../../lib/update/view';

  /**
   * Updates. The page fetches the three answers the panel derives from and polls while something is in flight; the
   * judgement is in `deriveUpdateView`, so the eight states are Vitest fixtures rather than screenshots taken at the
   * right moment. It checks quietly on open when nothing has been checked recently, which is the mockup's "Sepia
   * checks quietly. Nothing installs until you press the button."
   */
  const client = createClient();

  let version = $state<VersionInfo | null>(null);
  let available = $state<UpdateAvailable | null>(null);
  let status = $state<UpdateStatus | null>(null);
  let problem = $state('');
  let loading = $state(true);
  let working = $state(false);
  let now = $state(Date.now());
  let startedAt = $state<number | null>(null);
  let autoChecked = false;

  const view = $derived(deriveUpdateView({ version, available, status, startedAt, now }));

  async function refresh() {
    const [nextVersion, nextAvailable, nextStatus] = await Promise.all([
      client.getVersion(),
      client.getUpdateAvailable(),
      client.getUpdateStatus(),
    ]);
    version = nextVersion;
    available = nextAvailable;
    status = nextStatus;
    now = Date.now();
  }

  async function load() {
    loading = true;
    problem = '';
    try {
      await refresh();
      // A quiet check on open, at most once, and only when the helper is there. A check that fails is not an error
      // screen: the panel shows what it could not find out.
      const stale =
        !available?.available.checked_at ||
        Date.now() - new Date(available.available.checked_at).getTime() > 10 * 60_000;
      if (
        !autoChecked &&
        version?.updater.state === 'ready' &&
        stale &&
        !available?.check_pending
      ) {
        autoChecked = true;
        await client.checkForUpdates().catch(() => undefined);
        await refresh();
      }
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read the update state.';
    } finally {
      loading = false;
    }
  }

  async function act(run: () => Promise<unknown>) {
    working = true;
    problem = '';
    try {
      await run();
      await refresh();
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not ask for that.';
    } finally {
      working = false;
    }
  }

  function check() {
    return act(() => client.checkForUpdates());
  }

  function update() {
    startedAt = Date.now();
    return act(() => client.startUpdate());
  }

  function restart() {
    startedAt = Date.now();
    return act(() => client.restart());
  }

  $effect(() => {
    void load();
  });

  // While a check or an update is in flight the page polls; the rest of the time one timer only keeps the "still
  // working" line honest, and is cheap.
  $effect(() => {
    const timer = setInterval(() => {
      now = Date.now();
      if (view.kind === 'checking' || view.kind === 'updating')
        void refresh().catch(() => undefined);
    }, 2000);
    return () => clearInterval(timer);
  });

  // Done means the server reloaded under this page: an old client must never keep talking to a new server.
  $effect(() => {
    if (view.kind !== 'done' || startedAt === null) return;
    const timer = setTimeout(() => window.location.reload(), 1500);
    return () => clearTimeout(timer);
  });
</script>

<svelte:head>
  <title>Updates · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Updates" back />

  <div class="scroll">
    {#if loading}
      <SkeletonRows rows={3} label="Reading the update helper" />
    {:else}
      {#if problem}
        <Errbox
          message={problem}
          retryable
          onretry={() => (view.kind === 'failed' ? void update() : void check())}
        />
      {/if}

      <dl class="kv">
        <dt>Version</dt>
        <dd class="mono">{version?.version ?? ''}</dd>
        <dt>Installed</dt>
        <dd>{version?.deployment === 'docker' ? 'Docker' : 'Bare'}</dd>
        <dt>Update helper</dt>
        <dd>
          {#if version?.updater.state === 'ready'}
            Running
          {:else if version?.updater.state === 'stale'}
            Gone quiet
          {:else}
            Not installed
          {/if}
        </dd>
      </dl>

      <UpdatePanel
        {view}
        {now}
        busy={working}
        oncheck={() => void check()}
        onupdate={() => void update()}
        onrestart={view.kind === 'up_to_date' || view.kind === 'available' || view.kind === 'failed'
          ? () => void restart()
          : undefined}
      />
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
  .mono {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
    font-size: 0.8125rem;
    overflow-wrap: anywhere;
  }
</style>
