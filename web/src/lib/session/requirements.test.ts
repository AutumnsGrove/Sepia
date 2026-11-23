import { describe, expect, it } from 'vitest';
import type { Draft } from '../contract/events';
import { saveState, waitingOnAnswer } from './requirements';

function draft(patch: Partial<Draft>): Draft {
  return {
    version: 1,
    work: { title: 'The Lantern Keeper' },
    rating: { quarters: 18, scale: 'quarter_stars_20', favourite: false },
    body: 'the ending landed but it was too long',
    body_origin: 'extracted',
    completion: 'finished',
    blocks: {
      work: 'verified',
      rating: 'set',
      body: 'extracted',
      facts: 'empty',
      answers: 'empty',
    },
    ...patch,
  };
}

describe('what Save is waiting for', () => {
  it('is off with no card at all', () => {
    expect(saveState(null)).toEqual({ allowed: false, reason: '', missing: [] });
  });

  it('trusts the server, not its own arithmetic', () => {
    // The card carries a rating and words, but the server says it cannot confirm: the button follows the server.
    const state = saveState(draft({ can_confirm: false, missing: ['rating'] }));
    expect(state.allowed).toBe(false);
    expect(state.reason).toBe('Add a rating to save');
  });

  it('waits for the question she has not answered', () => {
    // The card is confirmable, but a question that carries her answer is still open: saving now would store the card
    // the model built before she tapped, and her answer would never reach it (issue #46).
    const state = saveState(draft({ can_confirm: true, missing: [] }), true);
    expect(state.allowed).toBe(false);
    expect(state.reason).toBe('Answer or skip the question first');
  });

  it('only waits for the questions that carry an answer', () => {
    expect(waitingOnAnswer(['describe'])).toBe(true);
    expect(waitingOnAnswer(['question_form'])).toBe(true);
    expect(waitingOnAnswer(['choose_candidate', 'choose_medium'])).toBe(false);
  });

  it('is on when the server says so, whatever the numbers look like', () => {
    const state = saveState(draft({ can_confirm: true, missing: [] }));
    expect(state.allowed).toBe(true);
    expect(state.reason).toBe('');
  });

  it('says both reasons when both are missing', () => {
    const state = saveState(draft({ can_confirm: false, missing: ['rating', 'body'] }));
    expect(state.reason).toBe('Add a rating and a few words of your own to save');
  });
});
