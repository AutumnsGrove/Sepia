import { describe, expect, it } from 'vitest';
import { updateFixture, updateNames } from './fixtures';
import { deriveUpdateView, sinceText, shortDigest, SLOW_AFTER_MS, HELPER_COMMAND } from './view';

/**
 * The Updates panel is a pure function, so the happy, bad and sad routes are these fixtures: every state the
 * mockup's page 05 draws, plus the two the mockup leaves to words (a check that could not run, and a rollback that
 * also failed).
 */

describe('the update view', () => {
  it('draws every state the gallery names', () => {
    const kinds = updateNames.map((name) => deriveUpdateView(updateFixture(name)).kind);
    expect(new Set(kinds)).toEqual(
      new Set([
        'loading',
        'no_updater',
        'checking',
        'up_to_date',
        'available',
        'updating',
        'done',
        'failed',
      ]),
    );
  });

  it('says where to look when the helper is not installed', () => {
    const view = deriveUpdateView(updateFixture('upd-no-helper'));
    expect(view).toMatchObject({
      kind: 'no_updater',
      helper: 'not_installed',
      command: HELPER_COMMAND,
    });
  });

  it('treats a stale heartbeat as gone quiet, not as no helper', () => {
    expect(deriveUpdateView(updateFixture('upd-helper-quiet'))).toMatchObject({
      kind: 'no_updater',
      helper: 'stale',
    });
  });

  it('shows a check in progress before it shows anything it found', () => {
    expect(deriveUpdateView(updateFixture('upd-checking'))).toEqual({
      kind: 'checking',
      version: 'r214.a1b2c3d',
    });
  });

  it('reports up to date with when it was checked', () => {
    const view = deriveUpdateView(updateFixture('upd-up-to-date'));
    expect(view.kind).toBe('up_to_date');
    if (view.kind !== 'up_to_date') return;
    expect(view.checkError).toBe('');
    expect(sinceText(view.checkedAt, updateFixture('upd-up-to-date').now)).toBe('2 minutes ago');
  });

  it('does not report up to date when the check could not run', () => {
    const view = deriveUpdateView(updateFixture('upd-check-failed'));
    expect(view).toMatchObject({ kind: 'up_to_date' });
    if (view.kind !== 'up_to_date') return;
    expect(view.checkError).toContain('GHCR is unreachable');
  });

  it('names the version that is ready', () => {
    expect(deriveUpdateView(updateFixture('upd-available'))).toMatchObject({
      kind: 'available',
      latest: 'r219.e4f5a6b',
      blocked: false,
    });
  });

  it('says the reply must finish before the button works', () => {
    expect(deriveUpdateView(updateFixture('upd-blocked'))).toMatchObject({
      kind: 'available',
      blocked: true,
    });
  });

  it('shows the target while it updates', () => {
    expect(deriveUpdateView(updateFixture('upd-updating'))).toMatchObject({
      kind: 'updating',
      target: 'r219.e4f5a6b',
      slow: false,
    });
  });

  it('says it is still working after ninety seconds', () => {
    expect(deriveUpdateView(updateFixture('upd-slow'))).toMatchObject({
      kind: 'updating',
      slow: true,
    });
    const snapshot = updateFixture('upd-updating');
    expect(
      deriveUpdateView({ ...snapshot, startedAt: snapshot.now - SLOW_AFTER_MS - 1 }),
    ).toMatchObject({ slow: true });
    expect(
      deriveUpdateView({ ...snapshot, startedAt: snapshot.now - SLOW_AFTER_MS }),
    ).toMatchObject({ slow: false });
  });

  it('confirms an update and then reloads', () => {
    expect(deriveUpdateView(updateFixture('upd-done'))).toEqual({
      kind: 'done',
      version: 'r214.a1b2c3d',
    });
  });

  it('shows a failed update with the reason and a retry when the old version is back', () => {
    const view = deriveUpdateView(updateFixture('upd-failed'));
    expect(view).toMatchObject({ kind: 'failed', rolledBack: true, retryable: true });
    if (view.kind !== 'failed') return;
    expect(view.detail).toContain('unknown key');
    // The box already says the previous version is back; the detail line is the reason, not the sentence again.
    expect(view.detail).not.toContain('previous version is running again');
  });

  it('does not offer a retry when the rollback failed too', () => {
    expect(deriveUpdateView(updateFixture('upd-rollback-failed'))).toMatchObject({
      kind: 'failed',
      rolledBack: false,
      retryable: false,
    });
  });

  it('is loading until both answers have arrived', () => {
    const first = updateFixture('upd-up-to-date');
    expect(deriveUpdateView({ ...first, available: null }).kind).toBe('loading');
    expect(deriveUpdateView({ ...first, version: null }).kind).toBe('loading');
  });
});

describe('sinceText', () => {
  const now = Date.parse('2026-11-15T12:00:00Z');
  const at = (seconds: number) => new Date(now - seconds * 1000).toISOString();

  it('rounds to something a person would say', () => {
    expect(sinceText(at(10), now)).toBe('just now');
    expect(sinceText(at(120), now)).toBe('2 minutes ago');
    expect(sinceText(at(3700), now)).toBe('an hour ago');
    expect(sinceText(at(4 * 3600), now)).toBe('4 hours ago');
    expect(sinceText(at(25 * 3600), now)).toBe('yesterday');
    expect(sinceText(at(3 * 86400), now)).toBe('3 days ago');
    expect(sinceText(at(20 * 86400), now)).toContain('on ');
  });

  it('is empty without an instant, and never negative when the clocks disagree', () => {
    expect(sinceText(null, now)).toBe('');
    expect(sinceText('not a date', now)).toBe('');
    expect(sinceText(new Date(now + 60_000).toISOString(), now)).toBe('just now');
  });
});

describe('shortDigest', () => {
  it('names a build when the registry gave no version', () => {
    expect(shortDigest(`ghcr.io/autumnsgrove/sepia@sha256:${'b'.repeat(64)}`)).toBe('bbbbbbb');
    expect(shortDigest('latest')).toBe('latest');
  });
});
