// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Pencil } from 'lucide-react';
import {
  getDexWallets,
  getMMAvailableBalances,
  multiFundingOptsForAsset,
  type DexAsset,
  type DexWalletState,
  type DexMarket,
  type MMBotConfig,
  type MMMarketReport,
} from '../../services/dcrdexApi';
import { CoinIcon } from './CoinIcon';
import { CexIcon } from './CexIcon';
import { DexMMPlacementsChart } from './DexMMPlacementsChart';
import { DexMMOracleTable } from './DexMMOracleTable';
import { DexMMQuickConfig } from './DexMMQuickConfig';
import { DexMMKnobs } from './DexMMKnobs';
import { DexMMAssetPane, type Available, type TransferRange } from './DexMMAssetPane';
import { DexMMManualConfig } from './DexMMManualConfig';
import { resolveAssetMeta, suggestedAllocation } from './dexMMAlloc';
import {
  applyQuick,
  buildBotConfig,
  defaultQuick,
  defaultWalletOptions,
  isQuickShaped,
  lotsPerLevelIncrement,
  quickFromDraft,
  type AssetFactors,
  type ConfigDraft,
  type QuickDraft,
} from './dexMMConfig';
import { apiError } from '../../utils/apiError';

// DexMMConfigStep is step 3 of the wizard: configure the strategy. It offers a
// Quick mode (sliders that derive symmetric placements) and an Advanced mode
// (manual gap strategy and placement tables). The left column holds how the bot
// quotes, the right column what it holds and the market; the actions sit below
// both, the same in either mode. Both modes build the same mm.BotConfig.
export const DexMMConfigStep = ({
  host,
  market,
  initial,
  editing,
  running = false,
  report,
  catalog,
  cexMarket,
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
  running?: boolean;
  report: MMMarketReport | null;
  catalog: DexAsset[];
  // cexMarket carries the exchange's minimum withdrawals (atoms) for the pair.
  cexMarket?: { baseMinWithdraw: number; quoteMinWithdraw: number };
  onChangeMarket: () => void;
  onChangeBotType: () => void;
  onSave: (cfg: MMBotConfig) => Promise<void>;
  onSaveAndStart: (cfg: MMBotConfig) => Promise<void>;
  onCancel: () => void;
}) => {
  const botType = initial.botType;
  const hasPlacements = botType !== 'simplearb';
  const cexBot = botType !== 'basicmm';

  // lotSizeUSD is the USD value of one lot; bisonw sizes the lots step from it.
  const lotSizeUSD =
    report && report.baseFiatRate > 0 && market.baseConvFactor > 0
      ? (market.lotSize / market.baseConvFactor) * report.baseFiatRate
      : 0;
  const lotsIncrement = lotsPerLevelIncrement(lotSizeUSD);

  // A saved bot opens on Quick when Quick could have made it, as in bisonw.
  const [mode, setMode] = useState<'quick' | 'advanced'>(!editing || isQuickShaped(initial) ? 'quick' : 'advanced');
  const [draft, setDraft] = useState<ConfigDraft>(initial);
  const [quick, setQuick] = useState<QuickDraft>(() => (editing ? quickFromDraft(initial) : defaultQuick()));
  // pristine is a new bot nobody has edited yet: its lots follow the market's
  // lots step once the market report arrives.
  const [pristine, setPristine] = useState(!editing);
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

  useEffect(() => {
    if (!pristine || lotsIncrement <= 1) return;
    const q = { ...quick, lotsPerLevel: String(lotsIncrement) };
    setQuick(q);
    setDraft((d) => applyQuick(d, q));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lotsIncrement]);

  // updateQuick changes a Quick value and regenerates the draft from it, as
  // bisonw's quickConfigUpdated does.
  const updateQuick = (patch: Partial<QuickDraft>) => {
    const q = { ...quick, ...patch };
    setPristine(false);
    setQuick(q);
    setDraft((d) => applyQuick(d, q));
  };

  // Moving to Quick approximates the manual placements and regenerates them, as
  // bisonw's switchToQuickConfig does.
  const switchMode = (m: 'quick' | 'advanced') => {
    if (m === 'quick' && mode !== 'quick') {
      const q = quickFromDraft(draft, lotsIncrement);
      setQuick(q);
      setDraft(applyQuick(draft, q));
    }
    setMode(m);
  };

  const edit = (fn: (d: ConfigDraft) => ConfigDraft) => {
    setPristine(false);
    setDraft(fn);
  };

  const setField = <K extends keyof ConfigDraft>(key: K, val: ConfigDraft[K]) => edit((d) => ({ ...d, [key]: val }));

  const setFactor = (side: 'baseFactors' | 'quoteFactors', key: keyof AssetFactors, val: string) =>
    edit((d) => ({ ...d, [side]: { ...d[side], [key]: val } }));

  // What each asset has free for this bot: the daemon's answer, which covers the
  // CEX and other bots, or the wallet balance before the bot is first saved.
  const [free, setFree] = useState<{ dex: Record<string, number>; cex: Record<string, number> } | null>(null);
  const [wallets, setWallets] = useState<DexWalletState[]>([]);
  useEffect(() => {
    let live = true;
    getMMAvailableBalances(host, market.baseID, market.quoteID)
      .then((b) => live && setFree({ dex: b.dexBalances ?? {}, cex: b.cexBalances ?? {} }))
      .catch(() => {});
    getDexWallets()
      .then((w) => live && setWallets(w))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [host, market.baseID, market.quoteID]);

  const availableFor = (id: number, conv: number): Available => {
    if (free) {
      const at = (m: Record<string, number>) => (m[id] !== undefined ? m[id] / conv : null);
      return { dex: at(free.dex), cex: at(free.cex) };
    }
    const w = wallets.find((x) => x.assetID === id);
    return { dex: w ? w.available : null, cex: null };
  };

  // The commit each asset needs, from the same projection the start dialog uses;
  // bisonw spans the Minimum Transfer slider from it.
  const projected = useMemo(
    () => suggestedAllocation(buildBotConfig(host, market, draft), market, report, catalog),
    [host, market, draft, report, catalog],
  );

  const transferRange = (id: number, isQuote: boolean): TransferRange | null => {
    const minWithdraw = isQuote ? cexMarket?.quoteMinWithdraw : cexMarket?.baseMinWithdraw;
    if (!minWithdraw || !projected) return null;
    const conv = isQuote ? market.quoteConvFactor : market.baseConvFactor;
    const total = (projected.dex[id] ?? 0) + (projected.cex[id] ?? 0);
    return {
      min: minWithdraw / conv,
      max: Math.max(2 * minWithdraw, total) / conv,
      decimals: Math.min(8, Math.round(Math.log10(conv))),
    };
  };

  const build = (): MMBotConfig | null => {
    if (
      hasPlacements &&
      draft.buys.every((p) => !(Math.floor(Number(p.lots)) > 0)) &&
      draft.sells.every((p) => !(Math.floor(Number(p.lots)) > 0))
    ) {
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
      setErr(apiError(e, 'Failed to save bot config'));
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
      setErr(apiError(e, 'Failed to save bot config'));
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4 max-w-6xl">
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
          <span className="font-medium">
            {botType === 'basicmm'
              ? 'Basic market maker'
              : botType === 'arbmm'
                ? 'Arb market maker'
                : 'Simple arbitrage'}
          </span>
          {draft.cexName && <CexIcon name={draft.cexName} className="h-4 w-4" />}
          {!editing && <Pencil className="h-3 w-3 text-muted-foreground" />}
        </button>
      </div>

      {/* Quick / Advanced toggle */}
      <div className="inline-flex rounded-lg border border-border p-0.5 text-sm">
        {(hasPlacements ? (['quick', 'advanced'] as const) : (['quick'] as const)).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => switchMode(m)}
            className={`px-3 py-1 rounded-md capitalize ${mode === m ? 'bg-primary text-primary-foreground' : 'hover:bg-muted/10'}`}
          >
            {m}
          </button>
        ))}
      </div>

      <div className="grid gap-6 lg:grid-cols-2 items-start">
        <div className="space-y-4 min-w-0">
          {mode === 'quick' ? (
            <DexMMQuickConfig
              botType={botType}
              quick={quick}
              lotSizeUSD={lotSizeUSD}
              lotsIncrement={lotsIncrement}
              onChange={updateQuick}
            />
          ) : (
            <DexMMManualConfig draft={draft} market={market} onChange={(patch) => edit((d) => ({ ...d, ...patch }))} />
          )}

          <DexMMKnobs draft={draft} onChange={(patch) => edit((d) => ({ ...d, ...patch }))} />

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
        </div>

        <div className="space-y-4 min-w-0">
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
            {([false, true] as const).map((isQuote) => {
              const id = isQuote ? market.quoteID : market.baseID;
              const side = isQuote ? 'quoteFactors' : 'baseFactors';
              return (
                <DexMMAssetPane
                  key={id}
                  symbol={isQuote ? market.quote : market.base}
                  isQuote={isQuote}
                  botType={botType}
                  isToken={!!resolveAssetMeta(catalog, id)?.isToken}
                  factors={draft[side]}
                  onFactor={(key, val) => setFactor(side, key, val)}
                  showTransfer={cexBot && draft.cexRebalance}
                  transfer={transferRange(id, isQuote)}
                  walletOpts={isQuote ? quoteOpts : baseOpts}
                  walletValue={isQuote ? draft.quoteWalletOptions : draft.baseWalletOptions}
                  onWalletChange={(v) => setField(isQuote ? 'quoteWalletOptions' : 'baseWalletOptions', v)}
                  breakdown={projected ? projected.breakdown[isQuote ? 'quote' : 'base'] : null}
                  fiatRate={(isQuote ? report?.quoteFiatRate : report?.baseFiatRate) ?? 0}
                  available={availableFor(id, isQuote ? market.quoteConvFactor : market.baseConvFactor)}
                />
              );
            })}
          </div>

          <DexMMOracleTable market={market} report={report} />
        </div>
      </div>

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      {running && (
        <p className="text-[11px] text-muted-foreground">
          This bot is running. Saving applies the change straight away and leaves its orders on the book, but the stored
          config is untouched until the bot is stopped and saved again.
        </p>
      )}

      <div
        role="group"
        aria-label="Bot actions"
        className="flex flex-wrap justify-end gap-2 border-t border-border/40 pt-4"
      >
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 rounded-lg border border-border text-sm hover:bg-muted/10"
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={doSave}
          className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
        >
          {busy ? 'Saving...' : running ? 'Apply to running bot' : editing ? 'Save changes' : 'Create bot'}
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
      </div>
    </div>
  );
};
