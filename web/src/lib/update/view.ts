import type { UpdateAvailable, UpdateStatus, VersionInfo } from '../api/client';

/**
 * The Updates panel's state is a pure function of what the page knows, so every one of its eight views is a fixture
 * rather than a screenshot taken at the right moment (docs/UPDATE.md, "The panel's states"). The page only fetches,
 * polls and calls actions; this file decides what she sees.
 */

export interface Snapshot {
  version: VersionInfo | null;
  available: UpdateAvailable | null;
  status: UpdateStatus | null;
  /** When this page asked for the update, for the "longer than expected" line. */
  startedAt: number | null;
  now: number;
  /** True while a reply is being written. The server refuses too; the button says why before the tap. */
  replyBusy?: boolean;
}

export type UpdateView =
  | { kind: 'loading' }
  | { kind: 'no_updater'; helper: 'not_installed' | 'stale'; command: string }
  | { kind: 'checking'; version: string }
  | { kind: 'up_to_date'; version: string; checkedAt: string | null; checkError: string }
  | { kind: 'available'; version: string; latest: string; blocked: boolean }
  | { kind: 'updating'; version: string; target: string; slow: boolean }
  | { kind: 'done'; version: string }
  | { kind: 'failed'; detail: string; rolledBack: boolean; retryable: boolean };

/** The line the no-helper state offers, which is the one that explains itself on the potato. */
export const HELPER_COMMAND = 'systemctl status sepia-update.path';

/**
 * 90 seconds is when "this takes a minute" becomes untrue. It is the panel's own clock: the watcher's result is the
 * only authority on whether it finished.
 */
export const SLOW_AFTER_MS = 90_000;

export function deriveUpdateView(snapshot: Snapshot): UpdateView {
  const { version, available, status, startedAt, now, replyBusy = false } = snapshot;
  if (!version || !available) return { kind: 'loading' };

  // A helper that is not installed, or has stopped beating, decides the view on its own: nothing else it wrote can
  // be trusted, and the panel's job is to say where to look (docs/UPDATE.md, "a result without a heartbeat").
  if (version.updater.state !== 'ready') {
    return {
      kind: 'no_updater',
      helper: version.updater.state === 'stale' ? 'stale' : 'not_installed',
      command: HELPER_COMMAND,
    };
  }

  const current = version.version;
  if (status?.pending) {
    return {
      kind: 'updating',
      version: current,
      target: available.available.latest || available.available.latest_digest || 'the new version',
      slow: startedAt !== null && now - startedAt > SLOW_AFTER_MS,
    };
  }

  if (status?.result) {
    switch (status.result.status) {
      case 'ok':
        return { kind: 'done', version: current };
      case 'skipped':
        return {
          kind: 'up_to_date',
          version: current,
          checkedAt: available.available.checked_at ?? null,
          checkError: '',
        };
      case 'failed': {
        const detail = status.result.detail ?? 'The update did not work.';
        // The watcher writes one of two sentences at the end of its detail. The panel's own box says that part in
        // plain words, so the line under it is only the technical reason: repeating the sentence twice reads as two
        // failures.
        const rolledBack = detail.includes('previous version is running again');
        const down = detail.includes('may be down');
        const technical = detail
          .replace(/\s*The previous version is running again\.\s*$/, '')
          .replace(/\s*The previous version did not come back either; Sepia may be down\.\s*$/, '')
          .trim();
        return { kind: 'failed', detail: technical || detail, rolledBack, retryable: !down };
      }
    }
  }

  if (available.check_pending) return { kind: 'checking', version: current };
  if (available.available.update_available) {
    return {
      kind: 'available',
      version: current,
      latest: available.available.latest || available.available.latest_digest || 'A newer version',
      blocked: replyBusy,
    };
  }
  return {
    kind: 'up_to_date',
    version: current,
    checkedAt: available.available.checked_at ?? null,
    checkError: available.available.error ?? '',
  };
}

/**
 * "Last checked 2 minutes ago", from the page's clock and the server's instant. The browser's clock may differ from
 * the potato's, so a future instant is "just now" rather than a negative age.
 */
export function sinceText(iso: string | null, now: number): string {
  if (!iso) return '';
  const at = new Date(iso).getTime();
  if (Number.isNaN(at)) return '';
  const seconds = Math.max(0, Math.round((now - at) / 1000));
  if (seconds < 90) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} minutes ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return hours === 1 ? 'an hour ago' : `${hours} hours ago`;
  const days = Math.round(hours / 24);
  if (days < 7) return days === 1 ? 'yesterday' : `${days} days ago`;
  return `on ${new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })}`;
}

/** The short digest suffix, for the rare case where the registry's version annotation is missing. */
export function shortDigest(digest: string): string {
  const hex = digest.split('sha256:')[1];
  return hex ? hex.slice(0, 7) : digest;
}
