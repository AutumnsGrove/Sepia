import type { BackupsInfo } from '../api/client';

/**
 * The Backups screen is a pure function of the receipts and the helper's heartbeat, so every state is a Vitest
 * fixture rather than a screenshot taken at the right moment. The page only fetches and asks; this file decides what
 * she sees, and it never turns "no receipt" into "backed up".
 */

export type HelperState = 'ready' | 'stale' | 'not_installed';
export type MirrorState = 'ok' | 'failed' | 'not_configured';

export interface RunRow {
  id: string;
  when: string;
  target: string;
  size: string;
  failed: boolean;
  detail: string;
}

export interface BackupsView {
  helper: HelperState;
  helperText: string;
  /** The line that explains where to look when the helper is not answering. */
  helperCommand: string;
  lastRun: string;
  snapshotSize: string;
  mirror: MirrorState;
  mirrorText: string;
  /** The sentence under the table when something went wrong. Empty when nothing did. */
  problem: string;
  pending: boolean;
  canRun: boolean;
  runs: RunRow[];
}

/** The command the no-helper state offers, which is the one that names the unit on the potato. */
export const HELPER_COMMAND = 'systemctl status sepia-backup.timer';

/** How many runs the screen lists before the history belongs to the plan, not the page. */
export const MAX_RUNS = 8;

const HELPER_TEXT: Record<HelperState, string> = {
  ready: 'Running',
  stale: 'Gone quiet',
  not_installed: 'Not installed',
};

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/**
 * formatWhen is local time on purpose: the receipt is UTC and the screen is hers, so "Today, 03:30" is the time the
 * timer actually fired where she is. The month and hour are built by hand rather than through toLocaleString, so a
 * test is not at the mercy of the machine's locale.
 */
export function formatWhen(iso: string, now: number): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return 'Unknown';
  const today = new Date(now);
  const days = Math.round(
    (new Date(at.getFullYear(), at.getMonth(), at.getDate()).getTime() -
      new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()) /
      86_400_000,
  );
  const day =
    days === 0 ? 'Today' : days === -1 ? 'Yesterday' : `${at.getDate()} ${MONTHS[at.getMonth()]}`;
  const hour = String(at.getHours()).padStart(2, '0');
  const minute = String(at.getMinutes()).padStart(2, '0');
  return `${day}, ${hour}:${minute}`;
}

/** humanBytes is the short size the table shows, in the decimal units a file manager uses. */
export function humanBytes(bytes: number): string {
  const kb = 1000;
  const mb = kb * 1000;
  const gb = mb * 1000;
  if (bytes >= gb) return `${(bytes / gb).toFixed(1)} GB`;
  if (bytes >= mb) return `${(bytes / mb).toFixed(1)} MB`;
  if (bytes >= kb) return `${Math.round(bytes / kb)} kB`;
  return `${bytes} B`;
}

/** The receipt target that carries the snapshots, and the one that carries the covers. */
const SNAPSHOT_TARGET = 'local:snapshot';
const DB_SUFFIX = '/db';
const COVERS_SUFFIX = '/covers';

/**
 * backupsSummary is the Settings row's one-line answer: the same fact the screen behind it leads with, short enough
 * for a row. "Not yet" is deliberately not "Backed up", because those are different promises.
 */
export function backupsSummary(view: BackupsView): string {
  if (view.helper === 'not_installed') return 'Not set up';
  if (view.lastRun === 'Never') return 'Not yet';
  if (view.mirror === 'failed') return 'Last run failed';
  const [day, time] = view.lastRun.split(', ');
  if (!time) return `Backed up ${view.lastRun}`;
  if (day === 'Today') return `Backed up ${time}`;
  if (day === 'Yesterday') return 'Backed up yesterday';
  return `Backed up ${day}`;
}

export function deriveBackupsView(info: BackupsInfo, now: number): BackupsView {
  const runs = info.runs ?? [];
  const newest = runs[0];
  const db = runs.find((run) => run.target.endsWith(DB_SUFFIX));
  const covers = runs.find((run) => run.target.endsWith(COVERS_SUFFIX));
  const snapshot = runs.find((run) => run.target === SNAPSHOT_TARGET);

  const helper = (info.helper.state ?? 'not_installed') as HelperState;

  // What happened, in the order it matters: a snapshot that failed, then the mirror, then the covers. A failed cover
  // mirror is still a warning, because the reviews are in the database and in the snapshot.
  let problem = '';
  if (snapshot?.status === 'failed') problem = snapshot.detail || 'The snapshot did not finish.';
  if (db?.status === 'failed') problem = db.detail || problem || 'The copy to R2 did not finish.';
  if (covers?.status === 'failed')
    problem = covers.detail || problem || 'The covers were not copied.';

  let mirror: MirrorState = 'not_configured';
  if (db) {
    if (db.status === 'ok' && covers?.status !== 'failed') mirror = 'ok';
    else if (db.status === 'failed' || covers?.status === 'failed') mirror = 'failed';
  }

  // One row per night, not per target: the covers run is the same run, and its outcome is already in problem above.
  const rows = runs
    .filter((run) => run.target.endsWith(DB_SUFFIX) && run.status !== 'skipped')
    .slice(0, MAX_RUNS)
    .map((run) => ({
      id: run.id,
      when: formatWhen(run.ran_at, now),
      target: run.target,
      size: run.bytes > 0 ? humanBytes(run.bytes) : '',
      failed: run.status === 'failed',
      detail: run.detail,
    }));

  return {
    helper,
    helperText: HELPER_TEXT[helper] ?? 'Not installed',
    helperCommand: HELPER_COMMAND,
    lastRun: newest ? formatWhen(newest.ran_at, now) : 'Never',
    snapshotSize: db && db.bytes > 0 ? humanBytes(db.bytes) : '',
    mirror,
    mirrorText: mirror === 'ok' ? 'Mirrored' : mirror === 'failed' ? 'Not copied' : 'Not set up',
    problem,
    pending: info.pending,
    canRun: helper !== 'not_installed' && !info.pending,
    runs: rows,
  };
}
