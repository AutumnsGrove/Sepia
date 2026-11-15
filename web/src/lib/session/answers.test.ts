import { describe, expect, it } from 'vitest';
import { describeAnswer, questionAnswer, skippedDescribe, skippedQuestions } from './answers';

/**
 * The wire shapes for the two interactions whose answers are a structure, not a single value. The agent's decoder is
 * the contract: `describeAnswer.Selected` and `questionAnswer.Answers`. The old client sent neither shape, so the
 * tests below are what keep a selected chip from vanishing and a typed answer from throwing.
 */

describe('describeAnswer', () => {
  it('groups the picked chips by category and drops the untouched ones', () => {
    expect(
      describeAnswer({ characters: ['memorable', 'multilayered'], setting: [], plot: ['twisty'] }),
    ).toEqual({
      selected: { characters: ['memorable', 'multilayered'], plot: ['twisty'] },
      skip_all: false,
    });
  });

  it('does not share the component array with the wire value', () => {
    const chosen = { characters: ['memorable'] };
    const value = describeAnswer(chosen);
    chosen.characters.push('flat');
    expect(value.selected.characters).toEqual(['memorable']);
  });

  it('sends skip_all, not an empty picks list, when she skips', () => {
    expect(skippedDescribe()).toEqual({ selected: {}, skip_all: true });
  });
});

describe('questionAnswer', () => {
  it('sends one entry per answered question and leaves the blanks to the agent', () => {
    expect(
      questionAnswer({ ending: 'stuck the landing', recommend: '  ', length: 'right' }),
    ).toEqual({
      answers: [
        { id: 'ending', value: 'stuck the landing' },
        { id: 'length', value: 'right' },
      ],
    });
  });

  it('trims what she typed and never sends an empty answer', () => {
    expect(questionAnswer({ recommend: '  read it twice  ', other: '' })).toEqual({
      answers: [{ id: 'recommend', value: 'read it twice' }],
    });
  });

  it('skips as an empty list, which the agent records as skipped', () => {
    expect(skippedQuestions()).toEqual({ answers: [] });
  });
});
