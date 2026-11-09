import { describe, expect, it } from 'vitest';
import { hasFirstMessage, stashFirstMessage, takeFirstMessage } from './handoff';

/**
 * The first message handoff. The interesting cases are the ones a navigation can get wrong: taking it twice after a
 * reconnect, and stashing nothing at all.
 */
describe('the first message', () => {
  it('is taken once and then gone', () => {
    stashFirstMessage('  finished the lantern keeper last night  ');
    expect(hasFirstMessage()).toBe(true);
    expect(takeFirstMessage()).toBe('finished the lantern keeper last night');
    expect(hasFirstMessage()).toBe(false);
    expect(takeFirstMessage()).toBe('');
  });

  it('is nothing when the composer was empty, so an empty message is never sent', () => {
    stashFirstMessage('   ');
    expect(hasFirstMessage()).toBe(false);
    expect(takeFirstMessage()).toBe('');
  });
});
