import { describe, expect, it } from 'vitest';
import { costWords } from './cost';

describe('costs on the why sheet', () => {
  it('always shows what was consumed and what a plan covered', () => {
    expect(costWords({ total: 0.0021, covered: 0.0021 })).toEqual({
      consumed: '$0.0021 consumed',
      allowance: '$0.0021 of it covered by your plan',
    });
  });

  it('says plainly when nothing was covered', () => {
    expect(costWords({ total: 0.0004, covered: 0 })).toEqual({
      consumed: '$0.0004 consumed',
      allowance: 'Paid for, not covered by a plan',
    });
  });

  it('never reports the covered part as money spent on top of the total', () => {
    const words = costWords({ total: 0.01, covered: 0.004 });
    expect(words.consumed).toBe('$0.01 consumed');
    expect(words.allowance).toContain('$0.0040');
    expect(words.allowance).not.toContain('$0.014');
  });
});
