<script lang="ts">
  import { resolve } from '$app/paths';
  import { ChevronRight } from '@lucide/svelte';
  import {
    ApiError,
    createClient,
    type ServicesInfo,
    type Settings,
    type UpdateAvailable,
    type VersionInfo,
  } from '../../lib/api/client';
  import Banner from '../../lib/components/Banner.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import Switch from '../../lib/components/Switch.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { countText } from '../../lib/state/reading';
  import { serviceRows, servicesSummary } from '../../lib/update/services';

  /**
   * Settings: the few things worth changing, because most are decided in config. The updates and services rows carry
   * the one-line answer the screens behind them give; a row that led to a page saying nothing would be worse than no
   * row at all.
   */
  const client = createClient();

  let settings = $state<Settings | null>(null);
  let deletedCount = $state<number | null>(null);
  let version = $state<VersionInfo | null>(null);
  let services = $state<ServicesInfo | null>(null);
  let update = $state<UpdateAvailable | null>(null);
  let loading = $state(true);
  let problem = $state('');
  let writeProblem = $state('');
  let saving = $state(false);

  async function load() {
    loading = true;
    problem = '';
    try {
      settings = await client.getSettings();
      // The count is the length of one page at the documented maximum, which is the whole list for one person.
      deletedCount = await client
        .listEntries({ deleted: true, limit: 100 })
        .then((page) => page.items.length)
        .catch(() => null);
      // The two summary rows are the answer the screens behind them would give; a failure leaves the row quiet
      // rather than making the whole Settings screen an error.
      [version, services, update] = await Promise.all([
        client.getVersion().catch(() => null),
        client.getServices().catch(() => null),
        client.getUpdateAvailable().catch(() => null),
      ]);
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your settings.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });

  /** The updates row's value: what the watcher and the running version say, in one phrase. */
  function updatesText(info: VersionInfo | null, checked: UpdateAvailable | null): string {
    if (!info) return '';
    if (info.updater.state !== 'ready') return 'Not set up';
    if (checked?.available.update_available) return 'Update ready';
    return 'Up to date';
  }

  async function setPreread(on: boolean) {
    if (!settings || saving) return;
    const before = settings.preread;
    writeProblem = '';
    settings = { ...settings, preread: on };
    saving = true;
    try {
      settings = await client.putSettings({ preread: on });
    } catch (error) {
      // The switch goes back to what is in force: a control that shows a value nobody stored is worse than an error.
      settings = { ...settings, preread: before };
      writeProblem = error instanceof ApiError ? error.copy : 'Sepia could not save that.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Settings · Sepia</title>
</svelte:head>

<main class="screen">
  <TopBar title="Settings" back />

  <div class="scroll">
    {#if problem}
      <Errbox message={problem} retryable onretry={() => void load()} />
    {:else if loading}
      <SkeletonRows rows={3} label="Reading your settings" />
    {:else}
      {#if writeProblem}
        <Errbox
          message={writeProblem}
          retryable
          onretry={() => void setPreread(!settings?.preread)}
        />
      {/if}

      <div class="group">
        <p class="lbl"><span>Sepia</span></p>
        <a class="srow link" href={resolve('/settings/updates')}>
          <span>Updates</span>
          <span class="v">
            {updatesText(version, update)}
            <ChevronRight size={18} aria-hidden="true" />
          </span>
        </a>
        <a class="srow link" href={resolve('/settings/services')}>
          <span>Services</span>
          <span class="v">
            {servicesSummary(serviceRows(services))}
            <ChevronRight size={18} aria-hidden="true" />
          </span>
        </a>
      </div>

      <div class="group">
        <p class="lbl"><span>Behaviour</span></p>
        <div class="srow">
          <span class="lblcol">
            <span>Quick pre-read</span>
            <span class="sub">
              Reads each message first to route it and skip questions you already answered. Off
              sends every message straight to the agent.
            </span>
          </span>
          <Switch
            checked={settings?.preread ?? true}
            label="Quick pre-read"
            disabled={saving}
            onchange={(on) => void setPreread(on)}
          />
        </div>
      </div>

      <div class="group">
        <p class="lbl"><span>Your data</span></p>
        <a class="srow link" href={resolve('/deleted')}>
          <span>Deleted entries</span>
          <span class="v">
            {deletedCount === null ? '' : countText(deletedCount)}
            <ChevronRight size={18} aria-hidden="true" />
          </span>
        </a>
      </div>

      <Banner
        >Backups and export arrive with your data screens. Everything else here is set in config.</Banner
      >
    {/if}
  </div>
</main>

<style>
  .group {
    margin-top: 0.5rem;
  }
  .srow {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.9rem;
    min-height: 56px;
    padding: 0.6rem 0;
    border-bottom: 1px solid var(--line);
    color: var(--text);
    text-decoration: none;
    font-size: 0.9375rem;
  }
  .srow.link:hover {
    color: var(--accent);
  }
  .lblcol {
    display: grid;
    gap: 0.15rem;
    min-width: 0;
  }
  .sub {
    font-size: 0.8125rem;
    color: var(--text-2);
    line-height: 1.45;
  }
  .v {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-3);
    font-size: 0.875rem;
  }
  .group + :global(.banner) {
    margin-top: 1.25rem;
  }
</style>
