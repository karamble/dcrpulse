// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexMarket, DexOrderOption, MMBotAssetConfig, MMBotConfig, MMCexStatus, MMGapStrategy } from '../../services/dcrdexApi';

// Bot types mirror bisonw v1.0.6: a DEX-only basic market maker, a simple
// DEX/CEX arbitrageur, and a market maker that hedges fills on a CEX.
export type BotType = 'basicmm' | 'simplearb' | 'arbmm';

export const BOT_TYPES: { id: BotType; label: string; desc: string; cex: boolean }[] = [
  { id: 'basicmm', label: 'Basic market maker', desc: 'Quote both sides of the DEX book around an oracle price.', cex: false },
  { id: 'simplearb', label: 'Simple arbitrage', desc: 'Trade only when a DEX/CEX price gap is profitable.', cex: true },
  { id: 'arbmm', label: 'Arb market maker', desc: 'Market make on the DEX, hedging fills on the CEX.', cex: true },
];

export const GAP_STRATEGIES: MMGapStrategy[] = ['percent-plus', 'percent', 'multiplier', 'absolute', 'absolute-plus'];

export const needsCex = (t: BotType): boolean => BOT_TYPES.find((b) => b.id === t)!.cex;

// botTypeOf reports the bot type of a saved config.
export const botTypeOf = (cfg?: MMBotConfig): BotType =>
  cfg?.simpleArbConfig ? 'simplearb' : cfg?.arbMarketMakingConfig ? 'arbmm' : 'basicmm';

// PlacementRow is one editable order level: lots and a factor (gapFactor for
// basic MM, multiplier for arb MM). Stored as strings while editing.
export interface PlacementRow {
  lots: string;
  factor: string;
}

// AssetFactors are the per-asset uiConfig tuning factors (bisonw BotAssetConfig),
// edited as strings. They size order reserves, the quote slippage buffer, the
// token swap-fee reserves, and the auto-rebalance transfer threshold.
export interface AssetFactors {
  swapFeeN: string;
  orderReservesFactor: string;
  slippageBufferFactor: string;
  transferFactor: string;
}

// ConfigDraft is the editable state shared by the Quick and Advanced config
// modes. Both modes mutate the same draft so switching between them is lossless.
export interface ConfigDraft {
  botType: BotType;
  cexName?: string;
  gapStrategy: MMGapStrategy;
  driftTolerance: string;
  profit: string;
  orderPersistence: string;
  profitTrigger: string;
  maxActiveArbs: string;
  numEpochs: string;
  buys: PlacementRow[];
  sells: PlacementRow[];
  baseWalletOptions: Record<string, string>;
  quoteWalletOptions: Record<string, string>;
  // uiConfig (persisted in the saved bot config so it round-trips):
  cexRebalance: boolean;
  simpleArbLots: string;
  baseFactors: AssetFactors;
  quoteFactors: AssetFactors;
}

// QuickDraft holds the simplified slider values. Percent fields are entered as
// whole percents and converted to fractions when deriving placements.
export interface QuickDraft {
  levelsPerSide: string;
  lotsPerLevel: string;
  profitPct: string;
  levelSpacingPct: string;
  matchBufferPct: string;
}

const num = (s: string): number => Number(s) || 0;
const clamp = (n: number, lo: number, hi: number): number => Math.min(hi, Math.max(lo, n));

const defaultFactor = (t: BotType): string => (t === 'arbmm' ? '1.5' : '0.02');

// defaultFactors seeds bisonw's per-asset uiConfig defaults (mmsettings.ts), so a
// default-config bot allocates and rebalances identically to the official UI.
export const defaultFactors = (): AssetFactors => ({
  swapFeeN: '50',
  orderReservesFactor: '1',
  slippageBufferFactor: '0.05',
  transferFactor: '0.1',
});

const factorsFromConfig = (c?: MMBotAssetConfig): AssetFactors =>
  c
    ? {
        swapFeeN: String(c.swapFeeN),
        orderReservesFactor: String(c.orderReservesFactor),
        slippageBufferFactor: String(c.slippageBufferFactor),
        transferFactor: String(c.transferFactor),
      }
    : defaultFactors();

const factorsToConfig = (f: AssetFactors): MMBotAssetConfig => ({
  swapFeeN: Math.max(0, Math.floor(num(f.swapFeeN))),
  orderReservesFactor: Math.max(0, num(f.orderReservesFactor)),
  slippageBufferFactor: clamp(num(f.slippageBufferFactor), 0, 1),
  transferFactor: clamp(num(f.transferFactor), 0, 1),
});

export const defaultDraft = (botType: BotType, cexName?: string): ConfigDraft => ({
  botType,
  cexName: needsCex(botType) ? cexName : undefined,
  gapStrategy: 'percent-plus',
  driftTolerance: '0.001',
  profit: '0.01',
  orderPersistence: '10',
  profitTrigger: '0.01',
  maxActiveArbs: '5',
  numEpochs: '10',
  buys: [{ lots: '1', factor: defaultFactor(botType) }],
  sells: [{ lots: '1', factor: defaultFactor(botType) }],
  baseWalletOptions: {},
  quoteWalletOptions: {},
  cexRebalance: true,
  simpleArbLots: '1',
  baseFactors: defaultFactors(),
  quoteFactors: defaultFactors(),
});

// defaultWalletOptions seeds a wallet's funding options from their defaults,
// skipping quote-only options on the base asset (mirrors bisonw's
// defaultWalletOptions). multisplit defaults on, so a fresh bot can fund
// multi-orders from large UTXOs without manual configuration.
export const defaultWalletOptions = (opts: DexOrderOption[], isQuote: boolean): Record<string, string> => {
  const out: Record<string, string> = {};
  for (const o of opts) {
    if (o.quoteAssetOnly && !isQuote) continue;
    out[o.key] = o.default;
  }
  return out;
};

export const defaultQuick = (): QuickDraft => ({
  levelsPerSide: '1',
  lotsPerLevel: '1',
  profitPct: '1',
  levelSpacingPct: '0.5',
  matchBufferPct: '50',
});

// draftFromConfig seeds an editable draft from a saved bot config (for editing).
export const draftFromConfig = (cfg: MMBotConfig): ConfigDraft => {
  const botType = botTypeOf(cfg);
  const d = defaultDraft(botType, cfg.cexName);
  d.baseWalletOptions = { ...(cfg.baseWalletOptions ?? {}) };
  d.quoteWalletOptions = { ...(cfg.quoteWalletOptions ?? {}) };
  const basic = cfg.basicMarketMakingConfig;
  const arb = cfg.arbMarketMakingConfig;
  const simple = cfg.simpleArbConfig;
  if (basic) {
    d.gapStrategy = basic.gapStrategy;
    d.driftTolerance = String(basic.driftTolerance);
    d.buys = basic.buyPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.gapFactor) }));
    d.sells = basic.sellPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.gapFactor) }));
  } else if (arb) {
    d.profit = String(arb.profit);
    d.driftTolerance = String(arb.driftTolerance);
    d.orderPersistence = String(arb.orderPersistence);
    d.buys = arb.buyPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.multiplier) }));
    d.sells = arb.sellPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.multiplier) }));
  } else if (simple) {
    d.profitTrigger = String(simple.profitTrigger);
    d.maxActiveArbs = String(simple.maxActiveArbs);
    d.numEpochs = String(simple.numEpochsLeaveOpen);
  }
  const ui = cfg.uiConfig;
  if (ui) {
    d.cexRebalance = ui.cexRebalance;
    if (ui.simpleArbLots !== undefined) d.simpleArbLots = String(ui.simpleArbLots);
    d.baseFactors = factorsFromConfig(ui.baseConfig);
    d.quoteFactors = factorsFromConfig(ui.quoteConfig);
  }
  return d;
};

// deriveQuickPlacements builds the symmetric per-side placements from the quick
// config, matching bisonw v1.0.6 quickConfigUpdated: each of levelsPerSide
// levels uses lotsPerLevel lots, with gapFactor = profit + levelSpacing*n for
// basic MM, or the (matchBuffer + 1) multiplier for arb MM.
export const deriveQuickPlacements = (botType: BotType, q: QuickDraft): PlacementRow[] => {
  const profit = num(q.profitPct) / 100;
  const spacing = num(q.levelSpacingPct) / 100;
  const multiplier = num(q.matchBufferPct) / 100 + 1;
  const levels = botType === 'simplearb' ? 1 : Math.max(1, Math.floor(num(q.levelsPerSide) || 1));
  const lots = Math.max(1, Math.floor(num(q.lotsPerLevel) || 1));
  const rows: PlacementRow[] = [];
  for (let n = 0; n < levels; n++) {
    const factor = botType === 'basicmm' ? profit + spacing * n : multiplier;
    rows.push({ lots: String(lots), factor: String(Number(factor.toFixed(6))) });
  }
  return rows;
};

const toPlacements = (rows: PlacementRow[]): { lots: number; factor: number }[] =>
  rows
    .map((p) => ({ lots: Math.max(0, Math.floor(num(p.lots))), factor: num(p.factor) }))
    .filter((p) => p.lots > 0);

// buildBotConfig assembles the bisonw mm.BotConfig from the draft. Allocation is
// intentionally omitted here: v1.0.6 takes allocation at start time via
// mm.StartConfig, not in the saved config.
export const buildBotConfig = (host: string, market: DexMarket, d: ConfigDraft): MMBotConfig => {
  const cfg: MMBotConfig = { host, baseID: market.baseID, quoteID: market.quoteID };
  if (d.botType === 'basicmm') {
    cfg.basicMarketMakingConfig = {
      gapStrategy: d.gapStrategy,
      driftTolerance: num(d.driftTolerance),
      buyPlacements: toPlacements(d.buys).map((p) => ({ lots: p.lots, gapFactor: p.factor })),
      sellPlacements: toPlacements(d.sells).map((p) => ({ lots: p.lots, gapFactor: p.factor })),
    };
  } else {
    cfg.cexName = d.cexName;
    if (d.botType === 'simplearb') {
      cfg.simpleArbConfig = {
        profitTrigger: num(d.profitTrigger),
        maxActiveArbs: Math.max(1, Math.floor(num(d.maxActiveArbs) || 1)),
        numEpochsLeaveOpen: Math.max(2, Math.floor(num(d.numEpochs) || 2)),
      };
    } else {
      cfg.arbMarketMakingConfig = {
        profit: num(d.profit),
        driftTolerance: num(d.driftTolerance),
        orderPersistence: Math.max(2, Math.floor(num(d.orderPersistence) || 2)),
        buyPlacements: toPlacements(d.buys).map((p) => ({ lots: p.lots, multiplier: p.factor })),
        sellPlacements: toPlacements(d.sells).map((p) => ({ lots: p.lots, multiplier: p.factor })),
      };
    }
  }
  if (Object.keys(d.baseWalletOptions).length) cfg.baseWalletOptions = d.baseWalletOptions;
  if (Object.keys(d.quoteWalletOptions).length) cfg.quoteWalletOptions = d.quoteWalletOptions;
  // Persist the per-asset tuning factors + rebalance toggle as bisonw's uiConfig
  // so they round-trip on edit and stay interoperable with the official UI.
  cfg.uiConfig = {
    baseConfig: factorsToConfig(d.baseFactors),
    quoteConfig: factorsToConfig(d.quoteFactors),
    cexRebalance: d.cexRebalance,
  };
  if (d.botType === 'simplearb') cfg.uiConfig.simpleArbLots = Math.max(1, Math.floor(num(d.simpleArbLots) || 1));
  return cfg;
};

// toAtoms converts a conventional amount string to atomic units using a market
// conversion factor.
export const toAtoms = (conv: number, val: string): number => Math.round((num(val)) * conv);

// cexSupportsMarket reports whether a CEX status lists a market for the pair,
// i.e. whether that CEX can arbitrage it. Mirrors bisonw v1.0.6
// cexMarketSupportFilter (scans cexStatus.markets for a matching base/quote).
export const cexSupportsMarket = (st: MMCexStatus | undefined, baseID: number, quoteID: number): boolean => {
  if (!st?.markets) return false;
  return Object.values(st.markets).some((m) => m.baseID === baseID && m.quoteID === quoteID);
};

// cexesSupportingMarket returns the configured CEX names that can arbitrage the
// pair. Only configured CEXes appear in the status map, so support implies
// configured.
export const cexesSupportingMarket = (
  cexes: Record<string, MMCexStatus>,
  baseID: number,
  quoteID: number,
): string[] => Object.keys(cexes).filter((name) => cexSupportsMarket(cexes[name], baseID, quoteID));

// cexMarketFor returns the CEX's market entry (with withdrawal minimums) for the
// pair, or undefined when the CEX is unconfigured or lacks the market. Used to
// floor the auto-rebalance transfer sizes.
export const cexMarketFor = (
  cexes: Record<string, MMCexStatus>,
  cexName: string | undefined,
  baseID: number,
  quoteID: number,
): { baseMinWithdraw: number; quoteMinWithdraw: number } | undefined => {
  if (!cexName) return undefined;
  const markets = cexes[cexName]?.markets;
  if (!markets) return undefined;
  const m = Object.values(markets).find((mk) => mk.baseID === baseID && mk.quoteID === quoteID);
  return m ? { baseMinWithdraw: m.baseMinWithdraw, quoteMinWithdraw: m.quoteMinWithdraw } : undefined;
};
