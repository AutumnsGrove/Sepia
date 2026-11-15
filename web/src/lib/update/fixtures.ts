import type { UpdateAvailable, UpdateStatus, VersionInfo } from '../api/client';
import type { Snapshot } from './view';

const NOW = Date.parse('2026-11-15T12:00:00Z');

function version(state: 'ready' | 'not_installed' | 'stale' = 'ready'): VersionInfo {
  return {
    version: 'r214.a1b2c3d',
    deployment: 'docker',
    updater: {
      state,
      heartbeat_at: state === 'not_installed' ? undefined : new Date(NOW - 30_000).toISOString(),
    },
  };
}

function available(
  overrides: Partial<UpdateAvailable['available']> = {},
  pending = false,
): UpdateAvailable {
  return {
    available: { current: 'r214.a1b2c3d', update_available: false, ...overrides },
    check_pending: pending,
  };
}

function status(overrides: Partial<UpdateStatus> = {}): UpdateStatus {
  return { pending: false, ...overrides };
}

function base(): Snapshot {
  return {
    version: version(),
    available: available({
      latest: 'r214.a1b2c3d',
      latest_digest: `ghcr.io/autumnsgrove/sepia@sha256:${'a'.repeat(64)}`,
      checked_at: new Date(NOW - 120_000).toISOString(),
      update_available: false,
    }),
    status: status(),
    startedAt: null,
    now: NOW,
  };
}

/** Every state the gallery draws, in the order docs/UPDATE.md lists them. */
export const updateNames = [
  'upd-loading',
  'upd-no-helper',
  'upd-helper-quiet',
  'upd-checking',
  'upd-up-to-date',
  'upd-check-failed',
  'upd-available',
  'upd-blocked',
  'upd-updating',
  'upd-slow',
  'upd-done',
  'upd-failed',
  'upd-rollback-failed',
] as const;

export type UpdateName = (typeof updateNames)[number];

export function updateFixture(name: UpdateName): Snapshot {
  switch (name) {
    case 'upd-loading':
      return { ...base(), version: null, available: null };
    case 'upd-no-helper':
      return { ...base(), version: version('not_installed') };
    case 'upd-helper-quiet':
      return { ...base(), version: version('stale') };
    case 'upd-checking':
      return { ...base(), available: available({ latest: undefined }, true) };
    case 'upd-up-to-date':
      return base();
    case 'upd-check-failed':
      return {
        ...base(),
        available: available({ error: 'GHCR is unreachable: could not resolve host ghcr.io.' }),
      };
    case 'upd-available':
      return {
        ...base(),
        available: available(
          {
            latest: 'r219.e4f5a6b',
            latest_digest: `ghcr.io/autumnsgrove/sepia@sha256:${'b'.repeat(64)}`,
            update_available: true,
          },
          false,
        ),
      };
    case 'upd-blocked':
      return { ...updateFixture('upd-available'), replyBusy: true };
    case 'upd-updating':
      return {
        ...base(),
        available: available({ latest: 'r219.e4f5a6b', update_available: true }),
        status: status({ pending: true }),
        startedAt: NOW - 20_000,
      };
    case 'upd-slow':
      return {
        ...base(),
        available: available({ latest: 'r219.e4f5a6b', update_available: true }),
        status: status({ pending: true }),
        startedAt: NOW - 150_000,
      };
    case 'upd-done':
      return {
        ...base(),
        status: status({
          result: {
            status: 'ok',
            detail: 'Updated to sepia@sha256:bbbbbbb.',
            target: `ghcr.io/autumnsgrove/sepia@sha256:${'b'.repeat(64)}`,
            finished_at: new Date(NOW - 10_000).toISOString(),
          },
        }),
      };
    case 'upd-failed':
      return {
        ...base(),
        status: status({
          result: {
            status: 'failed',
            detail:
              'The new version did not pass its health check: config: unknown key SEPIA_X. The previous version is running again.',
            target: `ghcr.io/autumnsgrove/sepia@sha256:${'c'.repeat(64)}`,
            finished_at: new Date(NOW - 10_000).toISOString(),
          },
        }),
      };
    case 'upd-rollback-failed':
      return {
        ...base(),
        status: status({
          result: {
            status: 'failed',
            detail:
              'Sepia did not start: port is already allocated. The previous version did not come back either; Sepia may be down.',
            target: `ghcr.io/autumnsgrove/sepia@sha256:${'c'.repeat(64)}`,
            finished_at: new Date(NOW - 10_000).toISOString(),
          },
        }),
      };
  }
}
