import { describe, expect, it } from 'vitest';
import type { DexAsset, DexMarket, MMBotConfig, MMMarketReport } from '../../services/dcrdexApi';
import { suggestedAllocation } from './dexMMAlloc';

const unit = (conversionFactor: number) => ({ conversionFactor }) as DexAsset['unitInfo'];
const catalog = [
  { id: 42, symbol: 'dcr', isAccountBased: false, unitInfo: unit(1e8), availableWallets: [] },
  { id: 0, symbol: 'btc', isAccountBased: false, unitInfo: unit(1e8), availableWallets: [] },
] as unknown as DexAsset[];
const market = {
  base: 'DCR',
  quote: 'BTC',
  baseID: 42,
  quoteID: 0,
  lotSize: 100_000_000,
  baseConvFactor: 1e8,
  quoteConvFactor: 1e8,
} as DexMarket;
const fees = { max: { swap: 3000, redeem: 2000, refund: 1000 }, estimated: { swap: 1500, redeem: 1000, refund: 500 } };
const report: MMMarketReport = { price: 0, oracles: [], baseFiatRate: 20, quoteFiatRate: 60000, baseFees: fees, quoteFees: fees };
const ui = {
  baseConfig: { swapFeeN: 50, orderReservesFactor: 1, slippageBufferFactor: 0.05, transferFactor: 0.1 },
  quoteConfig: { swapFeeN: 50, orderReservesFactor: 0.5, slippageBufferFactor: 0.05, transferFactor: 0.1 },
  cexRebalance: true,
};
const basic: MMBotConfig = {
  host: 'dex.example:7232',
  baseID: 42,
  quoteID: 0,
  uiConfig: ui,
  basicMarketMakingConfig: {
    gapStrategy: 'percent-plus',
    driftTolerance: 0.002,
    buyPlacements: [{ lots: 2, gapFactor: 0.02 }],
    sellPlacements: [{ lots: 3, gapFactor: 0.02 }],
  },
};

// atoms turns a conventional breakdown amount into atoms, as the allocation is.
const atoms = (v: number) => Math.round(v * 1e8);

describe('allocation breakdown', () => {
  it('itemises the same commit the start dialog suggests', () => {
    const s = suggestedAllocation(basic, market, report, catalog)!;
    const { base, quote } = s.breakdown;
    expect(base.lots).toBe(3);
    expect(quote.lots).toBe(2);
    expect(base.book).toBe(3);
    // Each asset pays its own fees here, so they count toward its total.
    expect(Math.abs(atoms(base.total) - s.dex[42])).toBeLessThanOrEqual(1);
    expect(Math.abs(atoms(quote.total) - s.dex[0])).toBeLessThanOrEqual(1);
    expect(quote.slippage).toBeGreaterThan(0);
    expect(base.slippage).toBe(0);
    expect(base.feeSymbol).toBe('dcr');
  });

  it('puts the CEX inventory of an arb bot in its own line', () => {
    const arb: MMBotConfig = {
      ...basic,
      cexName: 'Binance',
      basicMarketMakingConfig: undefined,
      arbMarketMakingConfig: {
        profit: 0.02,
        driftTolerance: 0.002,
        orderPersistence: 20,
        buyPlacements: [{ lots: 2, multiplier: 1 }],
        sellPlacements: [{ lots: 3, multiplier: 1 }],
      },
    };
    const s = suggestedAllocation(arb, market, report, catalog)!;
    expect(atoms(s.breakdown.base.cex)).toBe(s.cex[42]);
    expect(atoms(s.breakdown.quote.cex)).toBe(s.cex[0]);
  });

  it("sizes a simple arbitrage bot from its lots", () => {
    const simple: MMBotConfig = {
      ...basic,
      cexName: 'Binance',
      basicMarketMakingConfig: undefined,
      simpleArbConfig: { profitTrigger: 0.02, maxActiveArbs: 100, numEpochsLeaveOpen: 20 },
      uiConfig: { ...ui, simpleArbLots: 4 },
    };
    const { base, quote } = suggestedAllocation(simple, market, report, catalog)!.breakdown;
    expect(base.lots).toBe(4);
    expect(quote.lots).toBe(4);
  });
});
