import type { ServicesInfo } from '../api/client';

/**
 * The Services screen's copy. The server says what is configured and what each thing is; this file says what stops
 * working when it is not, because that is the sentence she actually needs. "ok" means configured: the API never makes
 * a live call, so the screen says "Ready" rather than promising a reachability it did not check.
 */

export type ServiceName = ServicesInfo['services'][number]['name'];

export interface ServiceRow {
  name: ServiceName;
  label: string;
  detail: string;
  state: 'ok' | 'degraded' | 'down' | 'not_configured';
  stateText: string;
  /** The outage copy, shown under the name only when the service is not ready. */
  cost: string;
}

const LABELS: Record<ServiceName, string> = {
  main_model: 'Main model',
  decisions: 'Decisions',
  tmdb: 'TMDB',
  open_library: 'Open Library',
  polaris: 'Polaris',
  database: 'Database',
  r2: 'Backups',
};

const COST: Record<ServiceName, string> = {
  main_model:
    'There is no model to read your message, so you fill the card in yourself. Your words are still saved.',
  decisions:
    'Messages go straight to the agent, with no quick pre-read to route them or skip answered questions.',
  tmdb: 'Films and TV are found by hand. Nothing already saved is affected.',
  open_library: 'Books are found by hand. Nothing already saved is affected.',
  polaris:
    'Web lookups are off. Everything else still works, and anything the APIs miss is logged by hand.',
  database: 'Sepia cannot read or save anything. This is the one that has to work.',
  r2: 'Backups are off. Your reviews are still on the potato, and nothing is lost.',
};

const STATE_TEXT: Record<ServiceRow['state'], string> = {
  ok: 'Ready',
  degraded: 'Degraded',
  down: 'Down',
  not_configured: 'Not set up',
};

export function serviceRows(info: ServicesInfo | null): ServiceRow[] {
  if (!info) return [];
  return info.services.map((service) => ({
    name: service.name,
    label: LABELS[service.name] ?? service.name,
    detail: service.detail ?? '',
    state: service.state,
    stateText: STATE_TEXT[service.state],
    cost: COST[service.name] ?? '',
  }));
}

/** The one-line summary the Settings row shows. */
export function servicesSummary(rows: ServiceRow[]): string {
  if (rows.length === 0) return '';
  const missing = rows.filter((row) => row.state !== 'ok');
  if (missing.length === 0) return 'All ready';
  if (missing.length === 1) return `${missing[0].label} is not set up`;
  return `${missing.length} need attention`;
}
