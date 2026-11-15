/**
 * The shapes the agent reads back. They are not the shapes the component holds: the agent wants the Describe picks
 * grouped by category and the questions as a list, and the old component sent `{tags}` and a map keyed by question
 * id, which the agent silently read as nothing (Describe) or failed to decode (questions). These functions are pure
 * so the mismatch has a test rather than a lost answer.
 */

/** Describe: category id to the chip values she picked; categories she skipped are left out. */
export function describeAnswer(chosen: Record<string, string[]>): {
  selected: Record<string, string[]>;
  skip_all: boolean;
} {
  const selected: Record<string, string[]> = {};
  for (const [category, values] of Object.entries(chosen)) {
    if (values.length > 0) selected[category] = [...values];
  }
  return { selected, skip_all: false };
}

/** Describe, skipped: nothing selected, and the rest of the questions are skipped too. */
export function skippedDescribe(): { selected: Record<string, string[]>; skip_all: boolean } {
  return { selected: {}, skip_all: true };
}

/** Questions: one entry per answered question, with the value she chose or typed. */
export function questionAnswer(answers: Record<string, string>): {
  answers: { id: string; value: string }[];
} {
  return {
    answers: Object.entries(answers)
      .filter(([, value]) => value.trim() !== '')
      .map(([id, value]) => ({ id, value: value.trim() })),
  };
}

/** Questions, skipped: every question is recorded as skipped by the agent. */
export function skippedQuestions(): { answers: { id: string; value: string }[] } {
  return { answers: [] };
}
