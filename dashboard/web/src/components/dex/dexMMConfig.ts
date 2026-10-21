// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexMarket, DexOrderOption, MMBotAssetConfig, MMBotConfig, MMCexStatus, MMGapStrategy } from '../../services/dcrdexApi';

// Bot types mirror bisonw v1.0.6: a DEX-only basic market maker, a simple
// DEX/CEX arbitrageur, and a market maker that hedges fills on a CEX.
export type BotType = 'basicmm' | 'simplearb' | 'arbmm';

export const BOT_TYPES: { id: BotType; label: string; desc: string; cex: boolean }[] = [
  { id: 'basicmm', label: 'Basic Market Maker', desc: 'Quote both sides of the DEX book around an oracle price.', cex: false },
  { id: 'arbmm', label: 'Market Maker + Arbitrage', desc: 'Market make on the DEX, hedging fills on the CEX.', cex: true },
  { id: 'simplearb', label: 'Basic Arbitrage', desc: 'Trade only when a DEX/CEX price gap is profitable.', cex: true },
];

export const botTypeLabel = (t: BotType): string => BOT_TYPES.find((b) => b.id === t)!.label;

export const GAP_STRATEGIES: MMGapStrategy[] = ['percent-plus', 'percent', 'multiplier', 'absolute', 'absolute-plus'];

// GAP_STRATEGY_INFO is bisonw's label and description of each gap strategy, in
// the order its settings page lists them.
export const GAP_STRATEGY_INFO: { id: MMGapStrategy; label: string; desc: string }[] = [
  {
    id: 'percent-plus',
    label: 'Percent Plus',
    desc: 'The gap starts at the break-even spread, which is the minimum spread at which a buy-sell combo produces profit, and then adds the specified spread, calculated as a percentage of the mid-gap price.',
  },
  { id: 'percent', label: 'Percent', desc: 'The gap is set to the specified percent of the mid-gap price.' },
  {
    id: 'absolute-plus',
    label: 'Absolute Plus',
    desc: 'The gap starts at the break-even spread, which is the minimum spread at which a buy-sell combo produces profit, and then adds the specified rate difference to both sides.',
  },
  {
    id: 'absolute',
    label: 'Absolute',
    desc: 'The spread is set to the specified value both left and right of the mid-gap, resulting in a total gap of twice the specified value.',
  },
  {
    id: 'multiplier',
    label: 'Multiplier',
    desc: 'The gap is set to the specified multiple of the break-even spread, which is the minimum spread at which a buy-sell combo produces profit.',
  },
];

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
  // driftTolerancePct is entered in percent; the bot config stores a fraction.
  driftTolerancePct: string;
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
export const toPct = (fraction: number): string => String(Number((fraction * 100).toFixed(6)));
export const fromPct = (pct: string): number => Number((num(pct) / 100).toFixed(8));

// KNOB_LIMITS bound the bot-wide settings. Drift tolerance stops at bisonw's
// own 1% validation limit, below the 2% its slider allows; order persistence
// starts at the 2 epochs it requires.
export const KNOB_LIMITS = {
  driftTolerancePct: { min: 0, max: 1 },
  orderPersistence: { min: 2, max: 40 },
} as const;

// ASSET_LIMITS are the slider ranges of bisonw's per-asset settings.
export const ASSET_LIMITS = {
  orderReservesFactor: { min: 0, max: 3 },
  slippageBufferFactor: { min: 0, max: 0.3 },
  swapFeeN: { min: 0, max: 1000 },
} as const;

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

// defaultDraft is a new bot's draft, its placements and profit generated from
// the default Quick values so the page shows what a save would store.
export const defaultDraft = (botType: BotType, cexName?: string, lotsIncrement = 1): ConfigDraft =>
  applyQuick(
    {
      botType,
      cexName: needsCex(botType) ? cexName : undefined,
      gapStrategy: 'percent-plus',
      driftTolerancePct: '0.2',
      profit: '0.02',
      orderPersistence: '20',
      profitTrigger: '0.02',
      // bisonw sends 100 and does not show the setting.
      maxActiveArbs: '100',
      numEpochs: '20',
      buys: [],
      sells: [],
      baseWalletOptions: {},
      quoteWalletOptions: {},
      cexRebalance: true,
      simpleArbLots: '1',
      baseFactors: defaultFactors(),
      quoteFactors: defaultFactors(),
    },
    defaultQuick(lotsIncrement),
  );

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

// QUICK_LIMITS are bisonw v1.0.6's Quick placement inputs (mmsettings.ts
// defaultProfit, defaultLevelSpacing, defaultMatchBuffer), in percent.
export const QUICK_LIMITS = {
  profitPct: { min: 0.1, max: 10 },
  levelSpacingPct: { min: 0.1, max: 2 },
  matchBufferPct: { min: 0, max: 100 },
} as const;

// lotsPerLevelIncrement is bisonw's lots step: about 100 USD of lots, at least 1.
export const lotsPerLevelIncrement = (lotSizeUSD: number): number =>
  lotSizeUSD > 0 ? Math.round(Math.max(1, 100 / lotSizeUSD)) : 1;

export const defaultQuick = (lotsIncrement = 1): QuickDraft => ({
  levelsPerSide: '1',
  lotsPerLevel: String(lotsIncrement),
  profitPct: '2',
  levelSpacingPct: '0.5',
  matchBufferPct: '0',
});

// applyQuick writes the Quick values into the draft, as bisonw's quick config
// does: profit is the arb profit or the simple-arb trigger, lots the simple-arb
// lots, and the other bots get symmetric generated placements.
export const applyQuick = (d: ConfigDraft, q: QuickDraft): ConfigDraft => {
  const profit = String(Number((num(q.profitPct) / 100).toFixed(6)));
  if (d.botType === 'simplearb') {
    return { ...d, profitTrigger: profit, simpleArbLots: String(Math.max(1, Math.floor(num(q.lotsPerLevel)))) };
  }
  const rows = deriveQuickPlacements(d.botType, q);
  const next: ConfigDraft = { ...d, buys: rows, sells: rows.map((r) => ({ ...r })) };
  if (d.botType === 'basicmm') next.gapStrategy = 'percent-plus';
  else next.profit = profit;
  return next;
};

// isQuickShaped reports whether a draft is what Quick generates, so a saved bot
// reopens on the Quick tab (bisonw isQuickPlacements plus percent-plus).
export const isQuickShaped = (d: ConfigDraft): boolean => {
  if (d.botType === 'simplearb') return true;
  if (d.botType === 'basicmm' && d.gapStrategy !== 'percent-plus') return false;
  if (d.buys.length === 0 || d.buys.length !== d.sells.length) return false;
  return d.buys.every((b, i) => num(b.lots) === num(d.sells[i].lots) && num(b.factor) === num(d.sells[i].factor));
};

// quickFromDraft approximates the Quick values of a draft, as bisonw's
// switchToQuickConfig does when the user moves from manual to quick.
export const quickFromDraft = (d: ConfigDraft, lotsIncrement = 1): QuickDraft => {
  const q = defaultQuick(lotsIncrement);
  const pct = (v: number) => String(Number((v * 100).toFixed(4)));
  if (d.botType === 'simplearb') {
    return {
      ...q,
      profitPct: pct(num(d.profitTrigger)),
      lotsPerLevel: String(Math.max(1, Math.floor(num(d.simpleArbLots)))),
    };
  }
  if (d.botType === 'arbmm') q.profitPct = pct(num(d.profit));
  const buys = toPlacements(d.buys);
  const sells = toPlacements(d.sells);
  if (buys.length === 0 || sells.length === 0) return q;
  const levels = Math.max(1, Math.floor((buys.length + sells.length) / 2));
  const factors = (ps: { factor: number }[]) => ps.map((p) => p.factor);
  if (d.botType === 'basicmm') {
    const best = (ps: { factor: number }[]) => Math.min(...factors(ps));
    const worst = (ps: { factor: number }[]) => Math.max(...factors(ps));
    q.profitPct = pct((best(buys) + best(sells)) / 2);
    if (levels > 1) {
      const range = (worst(buys) - best(buys) + (worst(sells) - best(sells))) / 2;
      q.levelSpacingPct = pct(range / (levels - 1));
    }
  } else {
    const all = [...factors(buys), ...factors(sells)];
    q.matchBufferPct = pct(all.reduce((a, b) => a + b, 0) / all.length - 1 || 0);
  }
  const lots = [...buys, ...sells].reduce((a, p) => a + p.lots, 0);
  q.levelsPerSide = String(levels);
  q.lotsPerLevel = String(Math.max(1, Math.round(lots / 2 / levels)));
  return q;
};

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
    d.driftTolerancePct = toPct(basic.driftTolerance);
    d.buys = basic.buyPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.gapFactor) }));
    d.sells = basic.sellPlacements.map((p) => ({ lots: String(p.lots), factor: String(p.gapFactor) }));
  } else if (arb) {
    d.profit = String(arb.profit);
    d.driftTolerancePct = toPct(arb.driftTolerance);
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

// GapKind is how a placement's gap is entered: a percent of the mid price, a
// rate, or a multiplier. Arb market maker placements are always multipliers.
export type GapKind = 'percent' | 'rate' | 'multiplier';

export const gapKind = (botType: BotType, strategy: MMGapStrategy): GapKind =>
  botType === 'arbmm' || strategy === 'multiplier'
    ? 'multiplier'
    : strategy === 'percent' || strategy === 'percent-plus'
      ? 'percent'
      : 'rate';

// gapShown is a stored gap factor as entered; gapStored is the reverse.
export const gapShown = (kind: GapKind, factor: string): string => (kind === 'percent' ? toPct(num(factor)) : factor);
export const gapStored = (kind: GapKind, shown: string): string =>
  kind === 'percent' ? String(fromPct(shown)) : String(num(shown));

// newPlacementError checks a placement before it joins a side, with bisonw's
// rules and messages; lots must also be positive.
export const newPlacementError = (kind: GapKind, side: PlacementRow[], lots: string, gap: string): string | null => {
  if (!(Math.floor(num(lots)) > 0)) return 'Lots must be greater than 0';
  const g = Number(gap);
  if (gap.trim() === '' || !Number.isFinite(g)) return 'Enter a gap';
  if (kind === 'multiplier' && (g < 1 || g > 100)) return 'Multiplier must be between 1 and 100';
  if (kind === 'rate' && g <= 0) return 'Rate must be greater than 0';
  if (kind === 'percent' && (g <= 0 || g > 10)) return 'Percent must be between 0 and 10';
  const stored = num(gapStored(kind, gap));
  if (side.some((r) => num(r.factor) === stored)) return 'Duplicate placement';
  return null;
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
      driftTolerance: fromPct(d.driftTolerancePct),
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
        driftTolerance: fromPct(d.driftTolerancePct),
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

// gapFactorLimits mirrors bisonw's per-strategy bounds on a basic market
// maker's gap factor (mm_basic.go validatePlacement).
const gapFactorLimits = (strategy: MMGapStrategy): [number, number] => {
  switch (strategy) {
    case 'multiplier':
      return [1, 100];
    case 'percent':
    case 'percent-plus':
      return [0, 0.1];
    default:
      return [0, Number.MAX_VALUE];
  }
};

// runningUpdateError mirrors the checks bisonw runs on a config pushed to a
// running bot and returns the first failure, or null.
//
// Checking here is not politeness. bisonw runs the market/CEX checks before it
// pauses the bot, so those fail cleanly, but the per-bot-type validation runs
// inside the paused section: a config it rejects there stops the bot, which
// cancels every order it had booked.
export const runningUpdateError = (oldCfg: MMBotConfig, next: MMBotConfig): string | null => {
  if (oldCfg.cexName && !next.cexName) return 'A running bot cannot have its exchange removed.';
  if (oldCfg.cexName && oldCfg.cexName !== next.cexName) return 'A running bot cannot change exchange.';
  if (botTypeOf(oldCfg) !== botTypeOf(next)) return 'A running bot cannot change type.';
  return configError(next);
};

// configError runs bisonw's per-bot-type validation (BasicMarketMakingConfig,
// ArbMarketMakerConfig, SimpleArbConfig validate) and returns the first
// failure, or null. Every save runs it: bisonw itself only checks a basic bot,
// and only when it starts.
export const configError = (next: MMBotConfig): string | null => {
  const basic = next.basicMarketMakingConfig;
  if (basic) {
    if (basic.driftTolerance < 0 || basic.driftTolerance > 0.01) {
      return 'Drift tolerance must be between 0% and 1%.';
    }
    if (!GAP_STRATEGIES.includes(basic.gapStrategy)) return `Unknown gap strategy ${basic.gapStrategy}.`;
    const [lo, hi] = gapFactorLimits(basic.gapStrategy);
    const label = GAP_STRATEGY_INFO.find((g) => g.id === basic.gapStrategy)?.label;
    const kind = gapKind('basicmm', basic.gapStrategy);
    const shown = (f: number) => `${gapShown(kind, String(f))}${kind === 'percent' ? '%' : ''}`;
    for (const [side, rows] of [
      ['buy', basic.buyPlacements],
      ['sell', basic.sellPlacements],
    ] as const) {
      const seen = new Set<number>();
      for (const p of rows) {
        if (seen.has(p.gapFactor)) return `Two ${side} placements share the gap ${shown(p.gapFactor)}.`;
        seen.add(p.gapFactor);
        if (p.gapFactor < lo || p.gapFactor > hi) {
          return `A ${side} placement's gap ${shown(p.gapFactor)} is outside the ${label} range.`;
        }
      }
    }
    return null;
  }

  const simple = next.simpleArbConfig;
  if (simple) {
    if (simple.profitTrigger <= 0 || simple.profitTrigger > 1) {
      return 'Profit threshold must be above 0% and at most 100%.';
    }
    if (simple.maxActiveArbs < 1) return 'At least one active arbitrage must be allowed.';
    if (simple.numEpochsLeaveOpen < 2) return 'Arbitrages must be left open for at least 2 epochs.';
    return null;
  }

  const arb = next.arbMarketMakingConfig;
  if (arb) {
    if (arb.buyPlacements.length === 0 && arb.sellPlacements.length === 0) return 'Add at least one placement.';
    if (arb.profit <= 0) return 'Profit threshold must be above 0%.';
    if (arb.driftTolerance < 0 || arb.driftTolerance > 0.01) return 'Drift tolerance must be between 0% and 1%.';
    if (arb.orderPersistence < 2) return 'Orders must be left open for at least 2 epochs.';
    return null;
  }

  return 'No bot settings to apply.';
};
