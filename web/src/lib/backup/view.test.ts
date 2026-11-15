import { describe, expect, it } from 'vitest';
import type { BackupsInfo } from '../api/client';
import { backupsSummary, deriveBackupsView, formatWhen, humanBytes, MAX_RUNS } from './view';

/**
 * One fixture per state of the screen: never run, a good night, a failed mirror, a failed snapshot, a helper that
 * stopped, a request in flight. The point of the derivation is that these are assertions rather than screenshots.
 */

const NOW = new Date(2026, 10, 15, 9, 0, 0).getTime();

function at(daysAgo: number, hour = 3, minute = 30): string {
  return new Date(2026, 10, 15 - daysAgo, hour, minute, 0).toISOString();
}

function receipt(
  id: string,
  target: string,
  status: 'ok' | 'failed' | 'skipped',
  bytes: number,
  ranAt = at(0),
  detail = '',
) {
  return { id, ran_at: ranAt, target, status, bytes, detail };
}

function info(overrides: Partial<BackupsInfo> = {}): BackupsInfo {
  return {
    helper: { state: 'ready', heartbeat_at: at(0, 3, 31) },
    pending: false,
    last: null,
    runs: [],
    ...overrides,
  } as BackupsInfo;
}

describe('formatWhen', () => {
  it('names today and yesterday rather than a date', () => {
    expect(formatWhen(at(0), NOW)).toBe('Today, 03:30');
    expect(formatWhen(at(1), NOW)).toBe('Yesterday, 03:30');
    expect(formatWhen(at(2), NOW)).toBe('13 Nov, 03:30');
  });

  it('does not invent a time for a broken timestamp', () => {
    expect(formatWhen('not a date', NOW)).toBe('Unknown');
  });
});

describe('humanBytes', () => {
  it('uses the short decimal form the screens show', () => {
    expect(humanBytes(0)).toBe('0 B');
    expect(humanBytes(900)).toBe('900 B');
    expect(humanBytes(4_200_000)).toBe('4.2 MB');
    expect(humanBytes(2_500_000_000)).toBe('2.5 GB');
  });
});

describe('deriveBackupsView', () => {
  it('says nothing has run rather than pretending', () => {
    const view = deriveBackupsView(info({ helper: { state: 'not_installed' } }), NOW);
    expect(view.lastRun).toBe('Never');
    expect(view.mirror).toBe('not_configured');
    expect(view.mirrorText).toBe('Not set up');
    expect(view.helper).toBe('not_installed');
    expect(view.canRun).toBe(false);
    expect(view.problem).toBe('');
    expect(view.runs).toEqual([]);
  });

  it('reports a good night from the receipts', () => {
    const view = deriveBackupsView(
      info({
        runs: [
          receipt('run-2:covers', 'r2sepia:sepia-backups/covers', 'ok', 300_000),
          receipt('run-2:db', 'r2sepia:sepia-backups/db', 'ok', 4_200_000),
          receipt('run-1:db', 'r2sepia:sepia-backups/db', 'ok', 4_100_000, at(1)),
        ],
        last: receipt('run-2:covers', 'r2sepia:sepia-backups/covers', 'ok', 300_000) as never,
      }),
      NOW,
    );
    expect(view.lastRun).toBe('Today, 03:30');
    expect(view.snapshotSize).toBe('4.2 MB');
    expect(view.mirror).toBe('ok');
    expect(view.mirrorText).toBe('Mirrored');
    expect(view.problem).toBe('');
    expect(view.canRun).toBe(true);
    expect(view.runs).toHaveLength(2);
    expect(view.runs[0]).toMatchObject({ when: 'Today, 03:30', size: '4.2 MB', failed: false });
  });

  it('says the copy to R2 failed and quotes the reason', () => {
    const view = deriveBackupsView(
      info({
        runs: [
          receipt('run-2:covers', 'r2sepia:sepia-backups/covers', 'ok', 0),
          receipt(
            'run-2:db',
            'r2sepia:sepia-backups/db',
            'failed',
            4_200_000,
            at(0),
            'AccessDenied',
          ),
        ],
      }),
      NOW,
    );
    expect(view.mirror).toBe('failed');
    expect(view.mirrorText).toBe('Not copied');
    expect(view.problem).toBe('AccessDenied');
    expect(view.runs[0].failed).toBe(true);
    expect(view.runs[0].size).toBe('4.2 MB');
  });

  it('keeps a failed snapshot visible even when the mirror of the old one worked', () => {
    const view = deriveBackupsView(
      info({
        runs: [
          receipt('run-2:covers', 'r2sepia:sepia-backups/covers', 'ok', 0),
          receipt('run-2:db', 'r2sepia:sepia-backups/db', 'ok', 4_200_000),
          receipt(
            'run-2:local',
            'local:snapshot',
            'failed',
            0,
            at(0),
            'a snapshot failed its integrity check',
          ),
        ],
      }),
      NOW,
    );
    expect(view.mirror).toBe('ok');
    expect(view.problem).toContain('integrity check');
  });

  it('distinguishes a helper that stopped from one that never ran', () => {
    const view = deriveBackupsView(info({ helper: { state: 'stale', heartbeat_at: at(3) } }), NOW);
    expect(view.helper).toBe('stale');
    expect(view.helperText).toBe('Gone quiet');
    expect(view.helperCommand).toBe('systemctl status sepia-backup.timer');
  });

  it('disables the button while a request is in flight', () => {
    const view = deriveBackupsView(info({ pending: true }), NOW);
    expect(view.pending).toBe(true);
    expect(view.canRun).toBe(false);
  });

  it('caps the history and drops the skipped receipts', () => {
    const runs = Array.from({ length: MAX_RUNS + 5 }, (_, i) => {
      if (i === 0)
        return receipt(`run-${i}:covers`, 'r2sepia:sepia-backups/covers', 'skipped', 0, at(0));
      return receipt(`run-${i}:db`, 'r2sepia:sepia-backups/db', 'ok', 1000, at(i));
    });
    const view = deriveBackupsView(info({ runs }), NOW);
    expect(view.runs).toHaveLength(MAX_RUNS);
    expect(view.runs.every((run) => run.target.endsWith('/db'))).toBe(true);
  });
});

describe('backupsSummary', () => {
  it('is the Settings row answer, and never claims a backup that did not happen', () => {
    const goodNight = deriveBackupsView(
      info({
        runs: [receipt('run-2:db', 'r2sepia:sepia-backups/db', 'ok', 4_200_000)],
      }),
      NOW,
    );
    expect(backupsSummary(goodNight)).toBe('Backed up 03:30');

    const failed = deriveBackupsView(
      info({
        runs: [receipt('run-2:db', 'r2sepia:sepia-backups/db', 'failed', 0)],
      }),
      NOW,
    );
    expect(backupsSummary(failed)).toBe('Last run failed');

    expect(backupsSummary(deriveBackupsView(info(), NOW))).toBe('Not yet');
    expect(
      backupsSummary(deriveBackupsView(info({ helper: { state: 'not_installed' } }), NOW)),
    ).toBe('Not set up');
  });
});
