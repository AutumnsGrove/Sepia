/**
 * The composer's rules, kept out of the component so each one can be tested without a browser. The limit is the
 * one in docs/STANDARDS.md: 8,000 characters of a user message, counted as characters (code points), which is what
 * the contract's maxLength counts, so an emoji is one character and not two.
 */
export const MAX_MESSAGE = 8000;

export type Problem = '' | 'empty' | 'too_long' | 'queue_full' | 'session_closed';

export interface ComposerInput {
  text: string;
  /** A turn is in flight. */
  working: boolean;
  /** How many messages the server says are waiting (0 or 1). */
  queue: number;
  /** The session has been saved or discarded: there is nothing left to type into. */
  closed: boolean;
}

export interface ComposerState {
  /** Whether the send button would do anything. The button is Stop while working, so this is the send state. */
  sendable: boolean;
  problem: Problem;
  /** The textarea is off, with a reason, while a queued message is waiting. */
  disabled: boolean;
  placeholder: string;
}

/** Count characters the way the contract does: code points, not UTF-16 units. */
export function characterCount(text: string): number {
  return [...text].length;
}

export function messageProblem(text: string): '' | 'empty' | 'too_long' {
  if (text.trim() === '') return 'empty';
  if (characterCount(text) > MAX_MESSAGE) return 'too_long';
  return '';
}

export function composerState(input: ComposerInput): ComposerState {
  if (input.closed) {
    return { sendable: false, problem: 'session_closed', disabled: true, placeholder: '' };
  }
  const problem = messageProblem(input.text);
  // One message may wait behind the turn in flight. A second is refused in place rather than sent and rejected.
  if (input.working && input.queue >= 1) {
    return {
      sendable: false,
      problem: 'queue_full',
      disabled: true,
      placeholder: 'Wait for the next reply',
    };
  }
  return {
    sendable: problem === '',
    problem,
    disabled: false,
    placeholder: input.working ? 'You can keep typing' : 'Say what you thought',
  };
}
