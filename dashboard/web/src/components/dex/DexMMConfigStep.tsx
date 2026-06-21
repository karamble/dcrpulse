// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Pencil, Plus, X } from 'lucide-react';
import {
  multiFundingOptsForAsset,
  type DexAsset,
  type DexMarket,
  type MMBotConfig,
  type MMGapStrategy,
  type MMMarketReport,
} from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { CexIcon } from './CexIcon';
import { fmtUsd } from './dexFormat';
import { DexMMPlacementsChart } from './DexMMPlacementsChart';
import { DexMMOracleTable } from './DexMMOracleTable';
import { DexMMWalletOptions } from './DexMMWalletOptions';
import {
  buildBotConfig,
  defaultQuick,
  defaultWalletOptions,
  deriveQuickPlacements,
  GAP_STRATEGIES,
  type AssetFactors,
  type ConfigDraft,
  type PlacementRow,
  type QuickDraft,
} from './dexMMConfig';

const inputCls =
  'px-2.5 py-1.5 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary';

const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <label className="flex flex-col gap-1">
    <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{label}</span>
    {children}
  </label>
);

const Slider = ({
  label,
  value,
  min,
  max,
  step,
  suffix,
  hint,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  min: number;
  max: number;
  step: number;
  suffix?: string;
  hint?: string;
  disabled?: boolean;
  onChange: (v: string) => void;
}) => (
  <div className={`space-y-1 ${disabled ? 'opacity-40' : ''}`}>
    <div className="flex items-center justify-between">
      <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{label}</span>
      <span className="font-mono text-sm">
        {value}
        {suffix}
      </span>
    </div>
    <input
      type="range"
      min={min}
      max={max}
      step={step}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className="w-full accent-primary"
    />
    {hint && <div className="text-[10px] text-muted-foreground">{hint}</div>}
  </div>
);

// DexMMConfigStep is step 3 of the wizard: configure the strategy. It offers a
// Quick mode (sliders that derive symmetric placements) and an Advanced mode
// (manual gap strategy and placement tables), with a live placements chart and
// the market's oracle table. Both modes build the same mm.BotConfig.
export const DexMMConfigStep = ({
  host,
  market,
  initial,
  editing,
  report,
  catalog,
  onChangeMarket,
  onChangeBotType,
  onSave,
  onSaveAndStart,
  onCancel,
}: {
  host: string;
  market: DexMarket;
  initial: ConfigDraft;
  editing: boolean;
  report: MMMarketReport | null;
  catalog: DexAsset[];
  onChangeMarket: () => void;
  onChangeBotType: () => void;
  onSave: (cfg: MMBotConfig) => Promise<void>;
  onSaveAndStart: (cfg: MMBotConfig) => Promise<void>;
  onCancel: () => void;
}) => {
  const botType = initial.botType;
  const hasPlacements = botType !== 'simplearb';
  const cexBot = botType !== 'basicmm';

  const [mode, setMode] = useState<'quick' | 'advanced'>(editing ? 'advanced' : 'quick');
  const [draft, setDraft] = useState<ConfigDraft>(initial);
  const [quick, setQuick] = useState<QuickDraft>(defaultQuick);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const baseOpts = useMemo(() => multiFundingOptsForAsset(catalog, market.baseID), [catalog, market.baseID]);
  const quoteOpts = useMemo(() => multiFundingOptsForAsset(catalog, market.quoteID), [catalog, market.quoteID]);

  // Seed the wallet funding options with their defaults (multisplit on) once the
  // catalog loads, preserving any values loaded from a saved config.
  useEffect(() => {
    if (!baseOpts.length && !quoteOpts.length) return;
    setDraft((d) => ({
      ...d,
      baseWalletOptions: { ...defaultWalletOptions(baseOpts, false), ...d.baseWalletOptions },
      quoteWalletOptions: { ...defaultWalletOptions(quoteOpts, true), ...d.quoteWalletOptions },
    }));
  }, [baseOpts, quoteOpts]);

  // applyQuick updates a quick field and recomputes the derived draft so the
  // Advanced tables stay in sync, mirroring v1.0.6 quickConfigUpdated.
  const applyQuick = (patch: Partial<QuickDraft>) => {
    const q = { ...quick, ...patch };
    setQuick(q);
    setDraft((d) => {
      if (botType === 'simplearb') {
        return { ...d, profitTrigger: String(Number(q.profitPct) / 100) };
      }
      const rows = deriveQuickPlacements(botType, q);
      const next: ConfigDraft = { ...d, buys: rows, sells: rows.map((r) => ({ ...r })) };
      if (botType === 'basicmm') next.gapStrategy = 'percent-plus';
      return next;
    });
  };

  const setField = <K extends keyof ConfigDraft>(key: K, val: ConfigDraft[K]) =>
    setDraft((d) => ({ ...d, [key]: val }));

  const setFactor = (side: 'baseFactors' | 'quoteFactors', key: keyof AssetFactors, val: string) =>
    setDraft((d) => ({ ...d, [side]: { ...d[side], [key]: val } }));

  const editPlacement = (side: 'buys' | 'sells', i: number, field: keyof PlacementRow, val: string) =>
    setDraft((d) => ({ ...d, [side]: d[side].map((p, idx) => (idx === i ? { ...p, [field]: val } : p)) }));

  const addPlacement = (side: 'buys' | 'sells') =>
    setDraft((d) => ({ ...d, [side]: [...d[side], { lots: '1', factor: botType === 'arbmm' ? '1.5' : '0.02' }] }));

  const removePlacement = (side: 'buys' | 'sells', i: number) =>
    setDraft((d) => ({ ...d, [side]: d[side].filter((_, idx) => idx !== i) }));

  // usdPerLot is the conventional USD value of one lot, for the quick-config hint.
  const usdPerLot = useMemo(() => {
    if (!report || report.baseFiatRate <= 0 || market.baseConvFactor <= 0) return 0;
    return (market.lotSize / market.baseConvFactor) * report.baseFiatRate;
  }, [report, market]);

  const build = (): MMBotConfig | null => {
    if (hasPlacements && draft.buys.every((p) => !(Math.floor(Number(p.lots)) > 0)) && draft.sells.every((p) => !(Math.floor(Number(p.lots)) > 0))) {
      setErr('Add at least one buy or sell placement with lots greater than zero.');
      return null;
    }
    return buildBotConfig(host, market, draft);
  };

  const doSave = async () => {
    const cfg = build();
    if (!cfg) return;
    setBusy(true);
    setErr(null);
    try {
      await onSave(cfg);
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to save bot config');
      setBusy(false);
    }
  };

  const doSaveAndStart = async () => {
    const cfg = build();
    if (!cfg) return;
    setBusy(true);
    setErr(null);
    try {
      await onSaveAndStart(cfg);
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to save bot config');
      setBusy(false);
    }
  };

  const renderPlacements = (title: string, side: 'buys' | 'sells') => {
    const list = draft[side];
    return (
      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">{title}</span>
          <button
            type="button"
            onClick={() => addPlacement(side)}
            className="p-1 rounded hover:bg-muted/20 text-muted-foreground"
            title="Add level"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>
        {list.map((p, i) => (
          <div key={i} className="flex items-center gap-2">
            <input
              value={p.lots}
              onChange={(e) => editPlacement(side, i, 'lots', e.target.value)}
              placeholder="lots"
              className={`${inputCls} w-20`}
            />
            <input
              value={p.factor}
              onChange={(e) => editPlacement(side, i, 'factor', e.target.value)}
              placeholder={botType === 'arbmm' ? 'multiplier' : 'gap'}
              className={`${inputCls} flex-1`}
            />
            {list.length > 1 && (
              <button
                type="button"
                onClick={() => removePlacement(side, i)}
                className="p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        ))}
      </div>
    );
  };

  return (
    <div className="space-y-4 max-w-2xl">
      {/* Market + bot type header, each editable */}
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={onChangeMarket}
          disabled={editing}
          className="flex items-center gap-2 text-sm hover:bg-muted/10 rounded-lg px-2 py-1 disabled:hover:bg-transparent"
        >
          <span className="flex -space-x-1.5">
            <CoinIcon symbol={market.base} className="h-5 w-5 ring-1 ring-card" />
            <CoinIcon symbol={market.quote} className="h-5 w-5 ring-1 ring-card" />
          </span>
          <span className="font-medium">
            {market.base}/{market.quote}
          </span>
          {!editing && <Pencil className="h-3 w-3 text-muted-foreground" />}
        </button>
        <span className="text-muted-foreground">/</span>
        <button
          type="button"
          onClick={onChangeBotType}
          disabled={editing}
          className="flex items-center gap-2 text-sm hover:bg-muted/10 rounded-lg px-2 py-1 disabled:hover:bg-transparent"
        >
          <span className="font-medium">{botType === 'basicmm' ? 'Basic market maker' : botType === 'arbmm' ? 'Arb market maker' : 'Simple arbitrage'}</span>
          {draft.cexName && <CexIcon name={draft.cexName} className="h-4 w-4" />}
          {!editing && <Pencil className="h-3 w-3 text-muted-foreground" />}
        </button>
      </div>

      {/* Quick / Advanced toggle */}
      <div className="inline-flex rounded-lg border border-border p-0.5 text-sm">
        {(['quick', 'advanced'] as const).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            className={`px-3 py-1 rounded-md capitalize ${mode === m ? 'bg-primary text-primary-foreground' : 'hover:bg-muted/10'}`}
          >
            {m}
          </button>
        ))}
      </div>

      {mode === 'quick' ? (
        <div className="space-y-4">
          {hasPlacements && (
            <Slider
              label="Levels per side"
              value={quick.levelsPerSide}
              min={1}
              max={10}
              step={1}
              onChange={(v) => applyQuick({ levelsPerSide: v })}
            />
          )}
          <Slider
            label={hasPlacements ? 'Lots per level' : 'Lots per arb'}
            value={quick.lotsPerLevel}
            min={1}
            max={50}
            step={1}
            hint={usdPerLot > 0 ? `~${fmtUsd(usdPerLot * Number(quick.lotsPerLevel))} per level` : undefined}
            onChange={(v) => applyQuick({ lotsPerLevel: v })}
          />
          <Slider
            label={botType === 'simplearb' ? 'Profit trigger' : 'Profit'}
            value={quick.profitPct}
            min={0}
            max={10}
            step={0.1}
            suffix="%"
            onChange={(v) => applyQuick({ profitPct: v })}
          />
          {botType === 'basicmm' && (
            <Slider
              label="Level spacing"
              value={quick.levelSpacingPct}
              min={0}
              max={5}
              step={0.1}
              suffix="%"
              disabled={Number(quick.levelsPerSide) <= 1}
              onChange={(v) => applyQuick({ levelSpacingPct: v })}
            />
          )}
          {botType === 'arbmm' && (
            <Slider
              label="Match buffer"
              value={quick.matchBufferPct}
              min={0}
              max={200}
              step={1}
              suffix="%"
              hint="Extra order size to ensure CEX fills cover DEX matches."
              onChange={(v) => applyQuick({ matchBufferPct: v })}
            />
          )}
        </div>
      ) : (
        <div className="space-y-4">
          <div className="grid sm:grid-cols-3 gap-3">
            {botType === 'basicmm' && (
              <Field label="Gap strategy">
                <select
                  value={draft.gapStrategy}
                  onChange={(e) => setField('gapStrategy', e.target.value as MMGapStrategy)}
                  className={inputCls}
                >
                  {GAP_STRATEGIES.map((g) => (
                    <option key={g} value={g}>
                      {g}
                    </option>
                  ))}
                </select>
              </Field>
            )}
            {botType === 'arbmm' && (
              <>
                <Field label="Profit">
                  <input value={draft.profit} onChange={(e) => setField('profit', e.target.value)} className={inputCls} />
                </Field>
                <Field label="Order persistence">
                  <input
                    value={draft.orderPersistence}
                    onChange={(e) => setField('orderPersistence', e.target.value)}
                    className={inputCls}
                  />
                </Field>
              </>
            )}
            {botType === 'simplearb' ? (
              <>
                <Field label="Profit trigger">
                  <input
                    value={draft.profitTrigger}
                    onChange={(e) => setField('profitTrigger', e.target.value)}
                    className={inputCls}
                  />
                </Field>
                <Field label="Max active arbs">
                  <input
                    value={draft.maxActiveArbs}
                    onChange={(e) => setField('maxActiveArbs', e.target.value)}
                    className={inputCls}
                  />
                </Field>
                <Field label="Epochs open">
                  <input value={draft.numEpochs} onChange={(e) => setField('numEpochs', e.target.value)} className={inputCls} />
                </Field>
              </>
            ) : (
              <Field label="Drift tolerance">
                <input
                  value={draft.driftTolerance}
                  onChange={(e) => setField('driftTolerance', e.target.value)}
                  className={inputCls}
                />
              </Field>
            )}
          </div>
          {hasPlacements && (
            <div className="grid sm:grid-cols-2 gap-4">
              {renderPlacements('Buy placements', 'buys')}
              {renderPlacements('Sell placements', 'sells')}
            </div>
          )}

          {/* Advanced allocation tuning (bisonw uiConfig per-asset factors) */}
          <div className="space-y-2 border-t border-border/40 pt-3">
            <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">Advanced allocation</span>
            <div className="grid sm:grid-cols-2 gap-3">
              <div className="space-y-2">
                <div className="flex items-center gap-1.5 text-xs font-medium">
                  <CoinIcon symbol={market.base} className="h-3.5 w-3.5" />
                  {market.base}
                </div>
                <Field label="Order reserves factor">
                  <input
                    value={draft.baseFactors.orderReservesFactor}
                    onChange={(e) => setFactor('baseFactors', 'orderReservesFactor', e.target.value)}
                    className={inputCls}
                  />
                </Field>
                <Field label="Swap fee reserves (N)">
                  <input
                    value={draft.baseFactors.swapFeeN}
                    onChange={(e) => setFactor('baseFactors', 'swapFeeN', e.target.value)}
                    className={inputCls}
                  />
                </Field>
              </div>
              <div className="space-y-2">
                <div className="flex items-center gap-1.5 text-xs font-medium">
                  <CoinIcon symbol={market.quote} className="h-3.5 w-3.5" />
                  {market.quote}
                </div>
                <Field label="Order reserves factor">
                  <input
                    value={draft.quoteFactors.orderReservesFactor}
                    onChange={(e) => setFactor('quoteFactors', 'orderReservesFactor', e.target.value)}
                    className={inputCls}
                  />
                </Field>
                <Field label="Swap fee reserves (N)">
                  <input
                    value={draft.quoteFactors.swapFeeN}
                    onChange={(e) => setFactor('quoteFactors', 'swapFeeN', e.target.value)}
                    className={inputCls}
                  />
                </Field>
                <Field label="Slippage buffer">
                  <input
                    value={draft.quoteFactors.slippageBufferFactor}
                    onChange={(e) => setFactor('quoteFactors', 'slippageBufferFactor', e.target.value)}
                    className={inputCls}
                  />
                </Field>
              </div>
            </div>
            {botType === 'simplearb' && (
              <Field label="Lots per arb">
                <input
                  value={draft.simpleArbLots}
                  onChange={(e) => setField('simpleArbLots', e.target.value)}
                  className={`${inputCls} w-28`}
                />
              </Field>
            )}
          </div>

          {/* Auto-rebalance (CEX bots): toggle + per-asset min-transfer factor */}
          {cexBot && (
            <div className="space-y-2 border-t border-border/40 pt-3">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={draft.cexRebalance}
                  onChange={(e) => setField('cexRebalance', e.target.checked)}
                  className="accent-primary"
                />
                <span>Auto-rebalance funds with the CEX</span>
              </label>
              {draft.cexRebalance && (
                <>
                  <div className="grid sm:grid-cols-2 gap-3">
                    <Slider
                      label={`${market.base} min transfer`}
                      value={draft.baseFactors.transferFactor}
                      min={0}
                      max={1}
                      step={0.01}
                      onChange={(v) => setFactor('baseFactors', 'transferFactor', v)}
                    />
                    <Slider
                      label={`${market.quote} min transfer`}
                      value={draft.quoteFactors.transferFactor}
                      min={0}
                      max={1}
                      step={0.01}
                      onChange={(v) => setFactor('quoteFactors', 'transferFactor', v)}
                    />
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    Transfers are floored at the CEX minimum withdrawal; the factor sizes the transfer up toward your free balance.
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      )}

      {hasPlacements && (
        <DexMMPlacementsChart
          market={market}
          botType={botType}
          gapStrategy={draft.gapStrategy}
          buys={draft.buys}
          sells={draft.sells}
          report={report}
        />
      )}

      <DexMMOracleTable market={market} report={report} />

      {(baseOpts.length > 0 || quoteOpts.length > 0) && (
        <div className="space-y-2">
          <span className="text-[11px] uppercase tracking-wider text-muted-foreground/70">Wallet options</span>
          <div className="grid sm:grid-cols-2 gap-3">
            <DexMMWalletOptions
              assetSymbol={market.base}
              opts={baseOpts}
              isQuote={false}
              value={draft.baseWalletOptions}
              onChange={(v) => setField('baseWalletOptions', v)}
            />
            <DexMMWalletOptions
              assetSymbol={market.quote}
              opts={quoteOpts}
              isQuote
              value={draft.quoteWalletOptions}
              onChange={(v) => setField('quoteWalletOptions', v)}
            />
          </div>
        </div>
      )}

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={doSave}
          className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
        >
          {busy ? 'Saving...' : editing ? 'Save changes' : 'Create bot'}
        </button>
        {!editing && (
          <button
            type="button"
            disabled={busy}
            onClick={doSaveAndStart}
            className="flex items-center gap-1.5 px-4 py-2 rounded-lg border border-border text-sm hover:bg-success/10 hover:text-success disabled:opacity-50"
          >
            Create &amp; start
          </button>
        )}
        <button type="button" onClick={onCancel} className="px-4 py-2 rounded-lg border border-border text-sm hover:bg-muted/10">
          Cancel
        </button>
      </div>
    </div>
  );
};
