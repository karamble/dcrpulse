// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { NumberSlider, Stepper } from './DexMMInputs';
import { KNOB_LIMITS, type ConfigDraft } from './dexMMConfig';

// DexMMKnobs are the bot-wide settings bisonw's settings page shows next to the
// placements on both tabs: drift tolerance, order persistence and the CEX
// rebalance switch, each only for the bot types that use it.
export const DexMMKnobs = ({
  draft,
  onChange,
}: {
  draft: ConfigDraft;
  onChange: (patch: Partial<ConfigDraft>) => void;
}) => {
  const { botType } = draft;
  const simpleArb = botType === 'simplearb';
  return (
    <div className="space-y-4 border-t border-border/40 pt-4">
      <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">Knobs</span>
      {botType !== 'basicmm' && (
        <label className="flex items-start gap-2 text-sm">
          <input
            type="checkbox"
            checked={draft.cexRebalance}
            onChange={(e) => onChange({ cexRebalance: e.target.checked })}
            className="mt-0.5 accent-primary"
          />
          <span>
            Arbitrage rebalance
            <span className="block text-[11px] text-muted-foreground">
              Transfer funds between wallet and CEX as needed and without prompt.
            </span>
          </span>
        </label>
      )}
      {!simpleArb && (
        <NumberSlider
          label="Drift tolerance"
          value={draft.driftTolerancePct}
          min={KNOB_LIMITS.driftTolerancePct.min}
          max={KNOB_LIMITS.driftTolerancePct.max}
          step={0.01}
          suffix="%"
          hint="How far from the ideal price orders are allowed to drift before they are cancelled and re-booked."
          onChange={(v) => onChange({ driftTolerancePct: v })}
        />
      )}
      {botType !== 'basicmm' && (
        <NumberSlider
          label="Order persistence"
          value={simpleArb ? draft.numEpochs : draft.orderPersistence}
          min={KNOB_LIMITS.orderPersistence.min}
          max={KNOB_LIMITS.orderPersistence.max}
          step={1}
          suffix="epochs"
          hint="How long CEX orders that don't immediately match are allowed to remain booked."
          onChange={(v) => onChange(simpleArb ? { numEpochs: v } : { orderPersistence: v })}
        />
      )}
      {simpleArb && (
        <Stepper
          label="Max active arbs"
          value={draft.maxActiveArbs}
          min={1}
          step={1}
          onChange={(v) => onChange({ maxActiveArbs: v })}
        />
      )}
    </div>
  );
};
