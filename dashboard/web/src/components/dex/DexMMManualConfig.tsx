// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { ArrowDown, ArrowUp, Plus, X } from 'lucide-react';
import type { DexMarket, MMGapStrategy } from '../../services/dcrdexApi';
import { Field, inputCls, NumberSlider } from './DexMMInputs';
import {
  fromPct,
  GAP_STRATEGY_INFO,
  gapKind,
  gapShown,
  gapStored,
  newPlacementError,
  QUICK_LIMITS,
  toPct,
  type ConfigDraft,
  type GapKind,
  type PlacementRow,
} from './dexMMConfig';

const gapHeader: Record<GapKind, string> = { percent: 'Percent', rate: 'Rate', multiplier: 'Multiplier' };

// PlacementTable is one side's placements in priority order, bisonw's buy or
// sell placements table: rows are reordered or removed, and new ones are
// checked before they are added.
const PlacementTable = ({
  side,
  rows,
  kind,
  header,
  unit,
  onChange,
}: {
  side: 'Buy' | 'Sell';
  rows: PlacementRow[];
  kind: GapKind;
  header: string;
  unit: string;
  onChange: (rows: PlacementRow[]) => void;
}) => {
  const [lots, setLots] = useState('');
  const [gap, setGap] = useState('');
  const [err, setErr] = useState<string | null>(null);

  const add = () => {
    const problem = newPlacementError(kind, rows, lots, gap);
    setErr(problem);
    if (problem) return;
    onChange([...rows, { lots: String(Math.floor(Number(lots))), factor: gapStored(kind, gap) }]);
    setLots('');
    setGap('');
  };
  const move = (i: number, by: number) => {
    const next = [...rows];
    [next[i], next[i + by]] = [next[i + by], next[i]];
    onChange(next);
  };
  const onEnter = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') add();
  };
  const iconBtn = 'p-1 rounded hover:bg-muted/20 text-muted-foreground';

  return (
    <div className="space-y-1.5">
      <div className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{side} placements</div>
      <p className="text-[10px] text-muted-foreground">
        The bot will place orders in the order of priority if balance is not enough to place all the orders.
      </p>
      <table aria-label={`${side} placements`} className="w-full text-xs">
        <thead>
          <tr className="text-muted-foreground/70 text-left">
            <th className="font-normal py-1 w-14">Priority</th>
            <th className="font-normal py-1">Lots</th>
            <th className="font-normal py-1">{header}</th>
            <th className="w-20" />
          </tr>
        </thead>
        <tbody className="font-mono">
          {rows.map((r, i) => (
            <tr key={`${r.factor}-${i}`} className="border-t border-border/30">
              <td className="py-1">{i + 1}</td>
              <td className="py-1">{r.lots}</td>
              <td className="py-1">
                {gapShown(kind, r.factor)} {unit}
              </td>
              <td className="py-1 text-right whitespace-nowrap">
                {i > 0 && (
                  <button type="button" aria-label="Move up" onClick={() => move(i, -1)} className={iconBtn}>
                    <ArrowUp className="h-3 w-3" />
                  </button>
                )}
                {i < rows.length - 1 && (
                  <button type="button" aria-label="Move down" onClick={() => move(i, 1)} className={iconBtn}>
                    <ArrowDown className="h-3 w-3" />
                  </button>
                )}
                <button
                  type="button"
                  aria-label="Remove"
                  onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
                  className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                >
                  <X className="h-3 w-3" />
                </button>
              </td>
            </tr>
          ))}
          <tr className="border-t border-border/30">
            <td className="py-1 text-muted-foreground">{rows.length + 1}</td>
            <td className="py-1 pr-2">
              <input
                type="number"
                min={1}
                step={1}
                aria-label={`${side} lots`}
                value={lots}
                onChange={(e) => setLots(e.target.value)}
                onKeyDown={onEnter}
                className={`${inputCls} w-full`}
              />
            </td>
            <td className="py-1 pr-2">
              <span className="flex items-center gap-1">
                <input
                  type="number"
                  aria-label={`${side} ${header.toLowerCase()}`}
                  value={gap}
                  onChange={(e) => setGap(e.target.value)}
                  onKeyDown={onEnter}
                  className={`${inputCls} w-full`}
                />
                <span className="text-muted-foreground font-sans whitespace-nowrap">{unit}</span>
              </span>
            </td>
            <td className="py-1 text-right">
              <button
                type="button"
                aria-label={`Add ${side.toLowerCase()} placement`}
                onClick={add}
                className={iconBtn}
              >
                <Plus className="h-3.5 w-3.5" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      {err && <div className="text-[11px] text-destructive">{err}</div>}
    </div>
  );
};

// DexMMManualConfig is the Advanced placements tab, bisonw v1.0.6's manual
// config: the gap strategy with its description, the arb profit, and the buy
// and sell placement tables, gaps entered in the strategy's own unit.
export const DexMMManualConfig = ({
  draft,
  market,
  onChange,
}: {
  draft: ConfigDraft;
  market: DexMarket;
  onChange: (patch: Partial<ConfigDraft>) => void;
}) => {
  const { botType, gapStrategy } = draft;
  // Placements are kept per strategy, as in bisonw: their gaps mean different
  // things, so a strategy change restores that strategy's own rows.
  const [byStrategy, setByStrategy] = useState<Partial<Record<MMGapStrategy, Pick<ConfigDraft, 'buys' | 'sells'>>>>({});
  const kind = gapKind(botType, gapStrategy);
  const unit = kind === 'percent' ? '%' : kind === 'multiplier' ? 'x' : `${market.quote}/${market.base}`;
  const header = botType === 'arbmm' ? 'Match buffer' : gapHeader[kind];
  const info = GAP_STRATEGY_INFO.find((g) => g.id === gapStrategy);

  const changeStrategy = (next: MMGapStrategy) => {
    setByStrategy((c) => ({ ...c, [gapStrategy]: { buys: draft.buys, sells: draft.sells } }));
    const saved = byStrategy[next];
    onChange({ gapStrategy: next, buys: saved?.buys ?? [], sells: saved?.sells ?? [] });
  };

  return (
    <div className="space-y-4">
      {botType === 'basicmm' && (
        <div className="space-y-1">
          <Field label="Gap strategy">
            <select
              value={gapStrategy}
              onChange={(e) => changeStrategy(e.target.value as MMGapStrategy)}
              className={inputCls}
            >
              {GAP_STRATEGY_INFO.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.label}
                </option>
              ))}
            </select>
          </Field>
          {info && <p className="text-[11px] text-muted-foreground">{info.desc}</p>}
        </div>
      )}
      {botType === 'arbmm' && (
        <NumberSlider
          label="Profit threshold"
          value={toPct(Number(draft.profit) || 0)}
          min={QUICK_LIMITS.profitPct.min}
          max={QUICK_LIMITS.profitPct.max}
          step={0.1}
          suffix="%"
          hint="The arbitrage bot will only attempt trades which would result in this minimum level of profit."
          onChange={(v) => onChange({ profit: String(fromPct(v)) })}
        />
      )}
      <PlacementTable
        side="Buy"
        rows={draft.buys}
        kind={kind}
        header={header}
        unit={unit}
        onChange={(buys) => onChange({ buys })}
      />
      <PlacementTable
        side="Sell"
        rows={draft.sells}
        kind={kind}
        header={header}
        unit={unit}
        onChange={(sells) => onChange({ sells })}
      />
    </div>
  );
};
