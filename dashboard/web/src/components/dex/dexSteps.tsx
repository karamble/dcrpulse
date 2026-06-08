// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Settlement step helpers shared by the order detail and the order panels. A DEX
// swap settles in four stages (maker swap, taker swap, maker redeem, taker
// redeem); the match status maps to a step index that drives the progress bar.

// stepIndex maps a match status to its settlement step (0..5). Mirrors bisonw's
// order.MatchStatus ordering: NewlyMatched 0, MakerSwapCast 1, TakerSwapCast 2,
// MakerRedeemed 3, MatchComplete 4, MatchConfirmed 5. The index is the stage
// currently confirming; lower stages are done, higher ones pending.
const STEP_BY_STATUS: Record<string, number> = {
  newlymatched: 0,
  makerswapcast: 1,
  takerswapcast: 2,
  makerredeemed: 3,
  matchcomplete: 4,
  matchconfirmed: 5,
};
export const stepIndex = (status: string): number => STEP_BY_STATUS[(status || '').toLowerCase()] ?? 0;

// orderStepIndex is an order's overall settlement stage: the least-progressed
// non-cancel match (the slowest one dictates completion). Null when the order has
// no matches yet (so callers can omit the bar).
export const orderStepIndex = (matches?: { isCancel: boolean; status: string }[]): number | null => {
  const ms = (matches || []).filter((m) => !m.isCancel);
  if (!ms.length) return null;
  return Math.min(...ms.map((m) => stepIndex(m.status)));
};

// StepBar renders the four settlement segments: green for a confirmed step
// (s < idx), orange for the one currently confirming (s === idx), muted for
// pending steps (s > idx).
export const StepBar = ({ idx, className = '' }: { idx: number; className?: string }) => (
  <span className={`flex gap-0.5 ${className}`}>
    {[1, 2, 3, 4].map((s) => (
      <span key={s} className={`h-1 flex-1 rounded ${s < idx ? 'bg-success' : s === idx ? 'bg-warning' : 'bg-muted/40'}`} />
    ))}
  </span>
);
