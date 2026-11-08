import { describe, expect, it } from 'vitest';
import { composerState, characterCount, messageProblem, MAX_MESSAGE } from './composer';

describe('the composer', () => {
  it('will not send an empty message', () => {
    expect(messageProblem('   ')).toBe('empty');
    expect(composerState({ text: '  ', working: false, queue: 0, closed: false }).sendable).toBe(
      false,
    );
  });

  it('counts characters the way the contract does, so an emoji is one', () => {
    expect(characterCount('👩🏽‍💻')).toBe(4); // the code points of the sequence, not the UTF-16 units
    expect(characterCount('a'.repeat(MAX_MESSAGE))).toBe(MAX_MESSAGE);
    expect(messageProblem('a'.repeat(MAX_MESSAGE))).toBe('');
    expect(messageProblem('a'.repeat(MAX_MESSAGE + 1))).toBe('too_long');
  });

  it('refuses over the limit in place rather than sending it', () => {
    const state = composerState({
      text: 'a'.repeat(MAX_MESSAGE + 1),
      working: false,
      queue: 0,
      closed: false,
    });
    expect(state.sendable).toBe(false);
    expect(state.problem).toBe('too_long');
    expect(state.disabled).toBe(false);
  });

  it('lets one message wait behind a turn, and refuses the second in place', () => {
    const waiting = composerState({ text: 'oh and imax', working: true, queue: 1, closed: false });
    expect(waiting.disabled).toBe(true);
    expect(waiting.problem).toBe('queue_full');
    expect(waiting.placeholder).toBe('Wait for the next reply');

    const first = composerState({ text: 'oh and imax', working: true, queue: 0, closed: false });
    expect(first.sendable).toBe(true);
    expect(first.placeholder).toBe('You can keep typing');
  });

  it('is off once the session has closed', () => {
    const state = composerState({ text: 'one more', working: false, queue: 0, closed: true });
    expect(state.disabled).toBe(true);
    expect(state.problem).toBe('session_closed');
  });
});
