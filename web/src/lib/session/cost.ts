/**
 * Money, said the way the protocol says it: what a call consumed and how much of that a plan's allowance covered.
 * The two are always shown together, because an allowance is not money spent and a screen that shows one number
 * could mean either.
 */
import { usdText } from './format';

export interface CostSummary {
  total: number;
  covered: number;
}

export function costWords(cost: CostSummary): { consumed: string; allowance: string } {
  const consumed = `${usdText(cost.total)} consumed`;
  if (cost.covered <= 0) return { consumed, allowance: 'Paid for, not covered by a plan' };
  if (cost.covered >= cost.total) {
    return { consumed, allowance: `${usdText(cost.covered)} of it covered by your plan` };
  }
  return { consumed, allowance: `${usdText(cost.covered)} of it covered by your plan` };
}
