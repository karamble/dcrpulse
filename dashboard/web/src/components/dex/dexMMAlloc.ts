// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexAsset, DexMarket, MMBotConfig, MMMarketReport } from '../../services/dcrdexApi';

// Faithful port of bisonw v1.0.6's market-maker funding/allocation math
// (client/webserver/site/src/js/mmutil.ts: calculateQuoteLot, feesAndCommit,
// projectedAllocations) so the funding dialog can suggest the same allocation
// bisonw would. The reserve factors below are bisonw's own defaults
// (mmsettings.ts), so a default-config bot sizes identically.
const ORDER_RESERVES_FACTOR = 1.0; // defaultOrderReserves.factor
const SLIPPAGE_BUFFER_FACTOR = 0.05; // defaultSlippage.factor
const SWAP_FEE_N = 50; // defaultSwapReserves.n
const RATE_ENCODING_FACTOR = 1e8; // OrderUtil.RateEncodingFactor

export interface AssetMeta {
  id: number;
  symbol: string;
  convFactor: number;
  // feeID is the asset that pays this asset's on-chain fees: the parent chain
  // for a token, otherwise the asset itself.
  feeID: number;
  feeConvFactor: number;
  isToken: boolean;
  // isAccountLocker mirrors bisonw's check on the fee asset's wallet (the
  // account-locker trait); account-based assets (e.g. ETH) are account-lockers.
  isAccountLocker: boolean;
}

// resolveAssetMeta finds an asset id in the catalog (a base coin or a nested
// token) and resolves its fee asset + factors.
export const resolveAssetMeta = (catalog: DexAsset[], assetID: number): AssetMeta | null => {
  for (const a of catalog) {
    if (a.id === assetID) {
      return {
        id: assetID,
        symbol: a.symbol,
        convFactor: a.unitInfo.conversionFactor,
        feeID: assetID,
        feeConvFactor: a.unitInfo.conversionFactor,
        isToken: false,
        isAccountLocker: a.isAccountBased,
      };
    }
    for (const t of a.tokens ?? []) {
      if (t.id === assetID) {
        return {
          id: assetID,
          symbol: t.symbol,
          convFactor: t.unitInfo.conversionFactor,
          feeID: t.parentID, // fees paid on the parent chain
          feeConvFactor: a.unitInfo.conversionFactor,
          isToken: true,
          isAccountLocker: a.isAccountBased, // the parent (e.g. ETH) is account-based
        };
      }
    }
  }
  return null;
};

const sumLots = (placements?: { lots: number }[]): number =>
  (placements ?? []).reduce((s, p) => s + (p.lots || 0), 0);

// lotsForConfig returns baseLots (total sell-side lots, which consume base
// inventory) and quoteLots (total buy-side lots, which consume quote), matching
// bisonw's BotMarket. simpleArb has no per-side placements; bisonw uses
// uiConfig.simpleArbLots (default 1), which we do not store.
const lotsForConfig = (cfg: MMBotConfig): { baseLots: number; quoteLots: number } => {
  if (cfg.basicMarketMakingConfig) {
    return {
      baseLots: sumLots(cfg.basicMarketMakingConfig.sellPlacements),
      quoteLots: sumLots(cfg.basicMarketMakingConfig.buyPlacements),
    };
  }
  if (cfg.arbMarketMakingConfig) {
    return {
      baseLots: sumLots(cfg.arbMarketMakingConfig.sellPlacements),
      quoteLots: sumLots(cfg.arbMarketMakingConfig.buyPlacements),
    };
  }
  return { baseLots: 1, quoteLots: 1 };
};

// calculateQuoteLot returns the quote-asset value (atomic) of one base lot,
// preferring fiat rates (as bisonw does) and falling back to the spot rate.
export const calculateQuoteLot = (
  lotSize: number,
  baseFactor: number,
  quoteFactor: number,
  baseFiatRate: number,
  quoteFiatRate: number,
  spotRate?: number,
): number => {
  if (baseFiatRate > 0 && quoteFiatRate > 0) {
    return ((lotSize * baseFiatRate) / quoteFiatRate) * (quoteFactor / baseFactor);
  }
  if (spotRate && spotRate > 0) {
    return (lotSize * spotRate) / RATE_ENCODING_FACTOR;
  }
  return quoteFactor;
};

// TRANSFER_FACTOR is bisonw's defaultTransfer.factor, used to size the minimum
// auto-rebalance transfer for CEX bots.
export const TRANSFER_FACTOR = 0.1;

export interface SuggestedAllocation {
  dex: Record<number, number>; // assetID -> atoms
  cex: Record<number, number>;
  // assets carries the symbol + conversion factor for every id present above so
  // the dialog can label and convert without re-resolving the catalog.
  assets: Record<number, { symbol: string; convFactor: number }>;
  // quoteLot is the quote-asset value (atomic) of one base lot, used to size the
  // quote-side auto-rebalance minimum.
  quoteLot: number;
}

// suggestedAllocation computes the funding allocation bisonw would propose for a
// saved bot config, given the market report (fees + fiat rates) and the asset
// catalog. Returns null when the fee data is not yet available. The DEX side
// gets the on-book inventory + order reserves + booking/swap fees; the CEX side
// gets the CEX inventory (only for arb bots). The totals equal bisonw's
// projectedAllocations; bisonw's interactive DEX/CEX rebalance split is a
// refinement not reproduced here.
export const suggestedAllocation = (
  cfg: MMBotConfig,
  market: DexMarket,
  report: MMMarketReport | null,
  catalog: DexAsset[],
): SuggestedAllocation | null => {
  const baseMeta = resolveAssetMeta(catalog, market.baseID);
  const quoteMeta = resolveAssetMeta(catalog, market.quoteID);
  if (!baseMeta || !quoteMeta) return null;
  const baseFees = report?.baseFees;
  const quoteFees = report?.quoteFees;
  if (!baseFees || !quoteFees) return null;

  const baseFactor = market.baseConvFactor;
  const quoteFactor = market.quoteConvFactor;
  const baseFeeFactor = baseMeta.feeConvFactor;
  const quoteFeeFactor = quoteMeta.feeConvFactor;
  const lotSize = market.lotSize;
  const lotSizeConv = lotSize / baseFactor;
  const quoteLot = calculateQuoteLot(
    lotSize,
    baseFactor,
    quoteFactor,
    report?.baseFiatRate ?? 0,
    report?.quoteFiatRate ?? 0,
    market.spot?.rate,
  );
  const quoteLotConv = quoteLot / quoteFactor;

  const { baseLots, quoteLots } = lotsForConfig(cfg);
  const hasCex = !!cfg.cexName;
  const { baseID, quoteID } = market;
  const baseFeeID = baseMeta.feeID;
  const quoteFeeID = quoteMeta.feeID;

  // ---- feesAndCommit ----
  const cexBaseLots = quoteLots;
  const cexQuoteLots = baseLots;
  const commit = {
    dex: { base: { lots: baseLots, val: baseLots * lotSize }, quote: { lots: quoteLots, val: quoteLots * quoteLot } },
    cex: { base: { lots: cexBaseLots, val: cexBaseLots * lotSize }, quote: { lots: cexQuoteLots, val: cexQuoteLots * quoteLot } },
  };

  let baseTokenFeesPerSwap = 0;
  let baseRedeemReservesPerLot = 0;
  if (baseID !== baseFeeID) {
    baseTokenFeesPerSwap += baseFees.estimated.swap;
    if (baseFeeID === quoteFeeID) baseTokenFeesPerSwap += quoteFees.estimated.redeem;
  }
  let baseBookingFeesPerLot = baseFees.max.swap;
  if (baseID === quoteFeeID) baseBookingFeesPerLot += quoteFees.max.redeem;
  if (baseMeta.isAccountLocker) {
    baseBookingFeesPerLot += baseFees.max.refund;
    if (!quoteMeta.isAccountLocker && baseFeeID !== quoteFeeID) baseRedeemReservesPerLot = baseFees.max.redeem;
  }

  let quoteTokenFeesPerSwap = 0;
  let quoteRedeemReservesPerLot = 0;
  if (quoteID !== quoteFeeID) {
    quoteTokenFeesPerSwap += quoteFees.estimated.swap;
    if (quoteFeeID === baseFeeID) quoteTokenFeesPerSwap += baseFees.estimated.redeem;
  }
  let quoteBookingFeesPerLot = quoteFees.max.swap;
  if (quoteID === baseFeeID) quoteBookingFeesPerLot += baseFees.max.redeem;
  if (quoteMeta.isAccountLocker) {
    quoteBookingFeesPerLot += quoteFees.max.refund;
    if (!baseMeta.isAccountLocker && quoteFeeID !== baseFeeID) quoteRedeemReservesPerLot = quoteFees.max.redeem;
  }

  const reservesFactor = 1 + ORDER_RESERVES_FACTOR;
  const baseBookingFees =
    baseBookingFeesPerLot * baseLots * reservesFactor + baseRedeemReservesPerLot * quoteLots * reservesFactor;
  const quoteBookingFees =
    quoteBookingFeesPerLot * quoteLots * reservesFactor + quoteRedeemReservesPerLot * baseLots * reservesFactor;

  // ---- projectedAllocations (conventional component amounts) ----
  const bBook = commit.dex.base.lots * lotSizeConv;
  const qBook = commit.cex.base.lots * quoteLotConv;
  const bOrderReserves = (Math.max(commit.cex.base.val, commit.dex.base.val) * ORDER_RESERVES_FACTOR) / baseFactor;
  const qOrderReserves = (Math.max(commit.cex.quote.val, commit.dex.quote.val) * ORDER_RESERVES_FACTOR) / quoteFactor;
  const bCex = hasCex ? commit.cex.base.lots * lotSizeConv : 0;
  const qCex = hasCex ? commit.cex.quote.lots * quoteLotConv : 0;
  const bBookingFees = baseBookingFees / baseFeeFactor;
  const qBookingFees = quoteBookingFees / quoteFeeFactor;
  const bSwapFeeReserves = baseMeta.isToken ? (baseTokenFeesPerSwap * SWAP_FEE_N) / baseFeeFactor : 0;
  const qSwapFeeReserves = quoteMeta.isToken ? (quoteTokenFeesPerSwap * SWAP_FEE_N) / quoteFeeFactor : 0;
  const qSlippage = (qBook + qCex + qOrderReserves) * SLIPPAGE_BUFFER_FACTOR;

  const dex: Record<number, number> = {};
  const cex: Record<number, number> = {};
  const add = (m: Record<number, number>, id: number, atoms: number) => {
    if (atoms > 0) m[id] = (m[id] ?? 0) + Math.round(atoms);
  };
  // DEX inventory + reserves + fees; fee asset entries merge when feeID == id.
  add(dex, baseID, (bBook + bOrderReserves) * baseFactor);
  add(dex, baseFeeID, (bBookingFees + bSwapFeeReserves) * baseFeeFactor);
  add(dex, quoteID, (qBook + qOrderReserves + qSlippage) * quoteFactor);
  add(dex, quoteFeeID, (qBookingFees + qSwapFeeReserves) * quoteFeeFactor);
  if (hasCex) {
    add(cex, baseID, bCex * baseFactor);
    add(cex, quoteID, qCex * quoteFactor);
  }

  // Carry the exact conversion factor each asset's atoms were computed with, so
  // the dialog's atoms -> conventional -> atoms round-trip is consistent (market
  // factors for the traded assets, catalog factors for token fee assets).
  const assets: Record<number, { symbol: string; convFactor: number }> = {
    [baseID]: { symbol: baseMeta.symbol, convFactor: baseFactor },
    [quoteID]: { symbol: quoteMeta.symbol, convFactor: quoteFactor },
  };
  if (baseFeeID !== baseID) {
    assets[baseFeeID] = { symbol: resolveAssetMeta(catalog, baseFeeID)?.symbol ?? String(baseFeeID), convFactor: baseFeeFactor };
  }
  if (quoteFeeID !== quoteID) {
    assets[quoteFeeID] = { symbol: resolveAssetMeta(catalog, quoteFeeID)?.symbol ?? String(quoteFeeID), convFactor: quoteFeeFactor };
  }

  return { dex, cex, assets, quoteLot };
};
