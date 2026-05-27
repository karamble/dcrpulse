// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexMarket, MMBotConfig, MMCexStatus, MMGapStrategy } from '../../services/dcrdexApi';

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

const defaultFactor = (t: BotType): string => (t === 'arbmm' ? '1.5' : '0.02');

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
});

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
