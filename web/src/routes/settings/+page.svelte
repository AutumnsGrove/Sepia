<script lang="ts">
  import { resolve } from '$app/paths';
  import { ChevronRight } from '@lucide/svelte';
  import { ApiError, createClient, type Settings } from '../../lib/api/client';
  import Banner from '../../lib/components/Banner.svelte';
  import Errbox from '../../lib/components/Errbox.svelte';
  import SkeletonRows from '../../lib/components/SkeletonRows.svelte';
  import Switch from '../../lib/components/Switch.svelte';
  import TopBar from '../../lib/components/TopBar.svelte';
  import { countText } from '../../lib/state/reading';

  /**
   * Settings: the few things worth changing, because most are decided in config. Today that is the quick pre-read
   * switch and the way into the Deleted list; Updates and Services arrive with S13, and neither is shown as a row that
   * does nothing.
   */
  const client = createClient();

  let settings = $state<Settings | null>(null);
  let deletedCount = $state<number | null>(null);
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
    } catch (error) {
      problem = error instanceof ApiError ? error.copy : 'Sepia could not read your settings.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });

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
        >Updates and services arrive with the updates screen. Backups arrive with your data screens.</Banner
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
