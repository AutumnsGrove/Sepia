import { describe, expect, it } from 'vitest';
import { serviceRows, servicesSummary } from './services';

/**
 * The Services copy: every state has a sentence, and an outage always says what stops working rather than only that
 * something is wrong.
 */

const info = {
  services: [
    { name: 'main_model' as const, state: 'ok' as const, detail: 'vendor/model' },
    { name: 'decisions' as const, state: 'ok' as const, detail: 'Quick pre-read and routing' },
    { name: 'tmdb' as const, state: 'not_configured' as const, detail: 'Films and TV' },
    { name: 'open_library' as const, state: 'ok' as const, detail: 'Books' },
    { name: 'polaris' as const, state: 'not_configured' as const, detail: 'Web lookups' },
    { name: 'database' as const, state: 'ok' as const, detail: 'Your entries and drafts' },
    { name: 'r2' as const, state: 'not_configured' as const, detail: 'Backups' },
  ],
};

describe('the services screen', () => {
  it('names every service and says what it is', () => {
    const rows = serviceRows(info);
    expect(rows.map((row) => row.label)).toEqual([
      'Main model',
      'Decisions',
      'TMDB',
      'Open Library',
      'Polaris',
      'Database',
      'Backups',
    ]);
    expect(rows[0].detail).toBe('vendor/model');
    expect(rows[2].stateText).toBe('Not set up');
  });

  it('says what an outage costs, in words rather than a colour', () => {
    const rows = serviceRows(info);
    for (const row of rows) expect(row.cost).not.toBe('');
    expect(rows.find((row) => row.name === 'polaris')?.cost).toContain('Web lookups are off');
  });

  it('summarises the settings row without hiding a missing service', () => {
    expect(servicesSummary(serviceRows(info))).toBe('3 need attention');
    const healthy = serviceRows({
      services: [{ name: 'main_model' as const, state: 'ok' as const }],
    });
    expect(servicesSummary(healthy)).toBe('All ready');
    expect(servicesSummary(serviceRows(null))).toBe('');
  });

  it('shows a service the server adds later rather than dropping it', () => {
    const rows = serviceRows({ services: [{ name: 'things' as never, state: 'ok' as const }] });
    expect(rows[0].label).toBe('things');
    expect(rows[0].cost).toBe('');
  });
});
