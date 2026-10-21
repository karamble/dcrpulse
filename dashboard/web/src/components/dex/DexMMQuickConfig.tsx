// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { ArrowLeftRight } from 'lucide-react';
import { fmtUsd } from './dexFormat';
import { NumberSlider, Stepper } from './DexMMInputs';
import { QUICK_LIMITS, type BotType, type QuickDraft } from './dexMMConfig';

// DexMMQuickConfig is the Quick placements tab, bisonw v1.0.6's quick config:
// levels, lots (or USD per side), profit threshold, and the price increment or
// match buffer, from which symmetric placements are generated.
export const DexMMQuickConfig = ({
  botType,
  quick,
  lotSizeUSD,
  lotsIncrement,
  onChange,
}: {
  botType: BotType;
  quick: QuickDraft;
  lotSizeUSD: number;
  lotsIncrement: number;
  onChange: (patch: Partial<QuickDraft>) => void;
}) => {
  const [usingUSD, setUsingUSD] = useState(false);
  const [usdText, setUsdText] = useState<string | null>(null);
  const simpleArb = botType === 'simplearb';
  const levels = simpleArb ? 1 : Math.max(1, Number(quick.levelsPerSide) || 1);
  const lots = Number(quick.lotsPerLevel) || 0;
  const usdPerSide = lots * levels * lotSizeUSD;

  // A typed USD amount stands until the lots or levels change elsewhere.
  useEffect(() => setUsdText(null), [quick.levelsPerSide, usingUSD]);

  const toggle = lotSizeUSD > 0 && (
    <button
      type="button"
      title={usingUSD ? 'Enter lots per level' : 'Enter USD per side'}
      aria-label={usingUSD ? 'Enter lots per level' : 'Enter USD per side'}
      onClick={() => setUsingUSD((u) => !u)}
      className="p-0.5 rounded hover:bg-muted/20 text-muted-foreground"
    >
      <ArrowLeftRight className="h-3 w-3" />
    </button>
  );
  const lotsLabel = simpleArb ? 'Lots per side' : 'Lots per level';

  return (
    <div className="space-y-4">
      {!simpleArb && (
        <Stepper
          label="Price levels per side"
          value={quick.levelsPerSide}
          min={1}
          step={1}
          onChange={(v) => onChange({ levelsPerSide: v })}
        />
      )}
      {usingUSD && lotSizeUSD > 0 ? (
        <Stepper
          label="USD per side"
          value={usdText ?? usdPerSide.toFixed(2)}
          min={Number(lotSizeUSD.toFixed(2))}
          step={Number((lotsIncrement * lotSizeUSD).toFixed(2))}
          action={toggle}
          hint={`~ ${lots} lots per level`}
          onChange={(v) => {
            setUsdText(v);
            const n = Number(v);
            if (n > 0) onChange({ lotsPerLevel: String(Math.max(1, Math.round(n / levels / lotSizeUSD))) });
          }}
        />
      ) : (
        <Stepper
          label={lotsLabel}
          value={quick.lotsPerLevel}
          min={1}
          step={lotsIncrement}
          action={toggle}
          hint={lotSizeUSD > 0 ? `~ ${fmtUsd(usdPerSide)} per side` : undefined}
          onChange={(v) => onChange({ lotsPerLevel: v })}
        />
      )}
      <NumberSlider
        label="Profit threshold"
        value={quick.profitPct}
        min={QUICK_LIMITS.profitPct.min}
        max={QUICK_LIMITS.profitPct.max}
        step={0.1}
        suffix="%"
        onChange={(v) => onChange({ profitPct: v })}
      />
      {botType === 'basicmm' && (
        <NumberSlider
          label="Price increment"
          value={quick.levelSpacingPct}
          min={QUICK_LIMITS.levelSpacingPct.min}
          max={QUICK_LIMITS.levelSpacingPct.max}
          step={0.1}
          suffix="%"
          disabled={levels <= 1}
          onChange={(v) => onChange({ levelSpacingPct: v })}
        />
      )}
      {botType === 'arbmm' && (
        <NumberSlider
          label="Match buffer"
          value={quick.matchBufferPct}
          min={QUICK_LIMITS.matchBufferPct.min}
          max={QUICK_LIMITS.matchBufferPct.max}
          step={1}
          suffix="%"
          hint="Extra order size to ensure CEX fills cover DEX matches."
          onChange={(v) => onChange({ matchBufferPct: v })}
        />
      )}
    </div>
  );
};
