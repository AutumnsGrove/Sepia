import type {
  ActivityRow,
  NoticeRow,
  OpenInteraction,
  SessionState,
  TranscriptBlock,
} from './session';

/**
 * One transcript, in the order things happened. The reducer keeps blocks, activity rows, notices and open cards in
 * separate lists because each has its own rules; the screen needs them interleaved, because an activity line
 * belongs under the message that caused it and before the card that answers it.
 *
 * The order is the reducer's own counter, not a timestamp: two events can share a second, and a replay must
 * rebuild the same screen.
 */
export type TimelineEntry =
  | { order: number; kind: 'block'; block: TranscriptBlock }
  | { order: number; kind: 'activity'; row: ActivityRow }
  | { order: number; kind: 'notice'; row: NoticeRow }
  | { order: number; kind: 'interaction'; interaction: OpenInteraction };

export function timeline(state: SessionState): TimelineEntry[] {
  const rows: TimelineEntry[] = [
    ...state.blocks.map((block) => ({ order: block.order, kind: 'block' as const, block })),
    ...state.activity.map((row) => ({ order: row.order, kind: 'activity' as const, row })),
    ...state.notices.map((row) => ({ order: row.order, kind: 'notice' as const, row })),
    ...state.interactions.map((interaction) => ({
      order: interaction.order,
      kind: 'interaction' as const,
      interaction,
    })),
  ];
  // A stable sort by the counter, then by kind, so an equal order (a hello's cards) still renders the same way.
  return rows.sort((a, b) => a.order - b.order || a.kind.localeCompare(b.kind));
}

/**
 * Whether a turn is in flight. A live activity row is the obvious sign, but the model can be streaming a reply with
 * no row at all, and a queued message is waiting behind a turn. Stop and the composer both hang off this, so it has
 * to be true for every one of those.
 */
export function isWorking(state: SessionState): boolean {
  if (state.status !== 'active') return false;
  if (state.queue > 0) return true;
  if (state.activity.some((row) => row.live)) return true;
  return state.blocks.some((block) => block.kind === 'assistant' && block.streaming);
}

/** The activity row still waiting to end, which is the one that pulses. */
export function liveActivity(state: SessionState): ActivityRow | null {
  for (let i = state.activity.length - 1; i >= 0; i -= 1) {
    if (state.activity[i].live) return state.activity[i];
  }
  return null;
}
