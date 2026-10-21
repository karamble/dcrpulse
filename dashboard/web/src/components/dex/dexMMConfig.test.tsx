import { describe, expect, it } from 'vitest';
import type { MMBotConfig } from '../../services/dcrdexApi';
import {
  applyQuick,
  configError,
  defaultDraft,
  defaultQuick,
  gapKind,
  gapShown,
  gapStored,
  isQuickShaped,
  lotsPerLevelIncrement,
  newPlacementError,
  quickFromDraft,
  runningUpdateError,
  type QuickDraft,
} from './dexMMConfig';

const quick = (q: Partial<QuickDraft>): QuickDraft => ({ ...defaultQuick(), ...q });

describe('Quick placements', () => {
  it('spaces basic market maker levels from the profit threshold', () => {
    const d = applyQuick(
      defaultDraft('basicmm'),
      quick({ levelsPerSide: '3', lotsPerLevel: '4', profitPct: '1.5', levelSpacingPct: '0.5' }),
    );
    expect(d.gapStrategy).toBe('percent-plus');
    expect(d.buys).toEqual([
      { lots: '4', factor: '0.015' },
      { lots: '4', factor: '0.02' },
      { lots: '4', factor: '0.025' },
    ]);
    expect(d.sells).toEqual(d.buys);
  });

  it('sets the arb market maker profit and match buffer', () => {
    const d = applyQuick(defaultDraft('arbmm', 'Binance'), quick({ profitPct: '3', matchBufferPct: '25' }));
    expect(d.profit).toBe('0.03');
    expect(d.buys).toEqual([{ lots: '1', factor: '1.25' }]);
  });

  it('sets the simple arbitrage trigger and lots', () => {
    const d = applyQuick(defaultDraft('simplearb', 'Binance'), quick({ profitPct: '0.7', lotsPerLevel: '6' }));
    expect(d.profitTrigger).toBe('0.007');
    expect(d.simpleArbLots).toBe('6');
  });

  it("saves a new bot's shown defaults", () => {
    const d = defaultDraft('basicmm', undefined, 3);
    expect(d.buys).toEqual([{ lots: '3', factor: '0.02' }]);
    expect(defaultQuick(3)).toMatchObject({ profitPct: '2', lotsPerLevel: '3', levelSpacingPct: '0.5', matchBufferPct: '0' });
  });

  it('sizes the lots step to about 100 USD', () => {
    expect(lotsPerLevelIncrement(12.5)).toBe(8);
    expect(lotsPerLevelIncrement(400)).toBe(1);
    expect(lotsPerLevelIncrement(0)).toBe(1);
  });
});

describe('reading a draft back into Quick', () => {
  it('recognises what Quick made, and only that', () => {
    const made = applyQuick(defaultDraft('basicmm'), quick({ levelsPerSide: '2' }));
    expect(isQuickShaped(made)).toBe(true);
    expect(isQuickShaped({ ...made, gapStrategy: 'percent' })).toBe(false);
    expect(isQuickShaped({ ...made, sells: [made.sells[0]] })).toBe(false);
    expect(isQuickShaped({ ...made, sells: [made.sells[0], { lots: '9', factor: made.sells[1].factor }] })).toBe(false);
  });

  it('recovers the Quick values of a basic market maker', () => {
    const q = quick({ levelsPerSide: '3', lotsPerLevel: '5', profitPct: '1.2', levelSpacingPct: '0.4' });
    expect(quickFromDraft(applyQuick(defaultDraft('basicmm'), q))).toMatchObject({
      levelsPerSide: '3',
      lotsPerLevel: '5',
      profitPct: '1.2',
      levelSpacingPct: '0.4',
    });
  });

  it('recovers the Quick values of the arbitrage bots', () => {
    const arb = applyQuick(defaultDraft('arbmm', 'Binance'), quick({ profitPct: '2.5', matchBufferPct: '40', lotsPerLevel: '2' }));
    expect(quickFromDraft(arb)).toMatchObject({ profitPct: '2.5', matchBufferPct: '40', lotsPerLevel: '2' });
    const simple = applyQuick(defaultDraft('simplearb', 'Binance'), quick({ profitPct: '0.9', lotsPerLevel: '7' }));
    expect(quickFromDraft(simple)).toMatchObject({ profitPct: '0.9', lotsPerLevel: '7' });
  });

  it('approximates manual placements the way bisonw does', () => {
    const d = {
      ...defaultDraft('basicmm'),
      buys: [
        { lots: '2', factor: '0.01' },
        { lots: '4', factor: '0.03' },
      ],
      sells: [{ lots: '3', factor: '0.02' }],
    };
    // One level per side from three placements; profit is the mean of the best
    // gaps; nine lots over two sides of one level round to five.
    expect(quickFromDraft(d)).toMatchObject({ levelsPerSide: '1', profitPct: '1.5', lotsPerLevel: '5' });
  });
});

describe('manual placements', () => {
  it("enters each strategy's gap in its own unit", () => {
    expect(gapKind('basicmm', 'percent-plus')).toBe('percent');
    expect(gapKind('basicmm', 'percent')).toBe('percent');
    expect(gapKind('basicmm', 'absolute-plus')).toBe('rate');
    expect(gapKind('basicmm', 'multiplier')).toBe('multiplier');
    expect(gapKind('arbmm', 'percent-plus')).toBe('multiplier');
    expect(gapStored('percent', '1.5')).toBe('0.015');
    expect(gapShown('percent', '0.015')).toBe('1.5');
    expect(gapStored('rate', '0.25')).toBe('0.25');
    expect(gapShown('multiplier', '2')).toBe('2');
  });

  it("refuses a placement bisonw would refuse", () => {
    const side = [{ lots: '1', factor: '0.02' }];
    expect(newPlacementError('percent', side, '0', '1')).toBe('Lots must be greater than 0');
    expect(newPlacementError('percent', side, '1', '')).toBe('Enter a gap');
    expect(newPlacementError('percent', side, '1', '0')).toBe('Percent must be between 0 and 10');
    expect(newPlacementError('percent', side, '1', '10.5')).toBe('Percent must be between 0 and 10');
    expect(newPlacementError('percent', side, '1', '2')).toBe('Duplicate placement');
    expect(newPlacementError('percent', side, '1', '10')).toBeNull();
    expect(newPlacementError('multiplier', [], '1', '0.9')).toBe('Multiplier must be between 1 and 100');
    expect(newPlacementError('multiplier', [], '1', '100')).toBeNull();
    expect(newPlacementError('rate', [], '1', '0')).toBe('Rate must be greater than 0');
  });
});

describe('checks on save', () => {
  const basic = (over: Partial<NonNullable<MMBotConfig['basicMarketMakingConfig']>>): MMBotConfig => ({
    host: 'dex.example:7232',
    baseID: 42,
    quoteID: 0,
    basicMarketMakingConfig: {
      gapStrategy: 'percent-plus',
      driftTolerance: 0.002,
      buyPlacements: [{ lots: 1, gapFactor: 0.02 }],
      sellPlacements: [{ lots: 1, gapFactor: 0.02 }],
      ...over,
    },
  });

  it("refuses what bisonw's validation refuses, in the units the page uses", () => {
    expect(configError(basic({}))).toBeNull();
    expect(configError(basic({ driftTolerance: 0.015 }))).toBe('Drift tolerance must be between 0% and 1%.');
    expect(configError(basic({ buyPlacements: [{ lots: 1, gapFactor: 0.15 }] }))).toBe(
      "A buy placement's gap 15% is outside the Percent Plus range.",
    );
    expect(
      configError(
        basic({
          sellPlacements: [
            { lots: 1, gapFactor: 0.02 },
            { lots: 2, gapFactor: 0.02 },
          ],
        }),
      ),
    ).toBe('Two sell placements share the gap 2%.');
    expect(configError(basic({ gapStrategy: 'multiplier' }))).toBe(
      "A buy placement's gap 0.02 is outside the Multiplier range.",
    );
    const arb: MMBotConfig = {
      host: 'dex.example:7232',
      baseID: 42,
      quoteID: 0,
      cexName: 'Binance',
      arbMarketMakingConfig: {
        profit: 0,
        driftTolerance: 0.002,
        orderPersistence: 20,
        buyPlacements: [{ lots: 1, multiplier: 1 }],
        sellPlacements: [],
      },
    };
    expect(configError(arb)).toBe('Profit threshold must be above 0%.');
    const simple: MMBotConfig = {
      host: 'dex.example:7232',
      baseID: 42,
      quoteID: 0,
      cexName: 'Binance',
      simpleArbConfig: { profitTrigger: 0, maxActiveArbs: 100, numEpochsLeaveOpen: 20 },
    };
    expect(configError(simple)).toBe('Profit threshold must be above 0% and at most 100%.');
  });
  it('applies the same checks to a running bot after its own', () => {
    expect(runningUpdateError(basic({}), basic({ driftTolerance: 0.015 }))).toBe(
      'Drift tolerance must be between 0% and 1%.',
    );
    expect(runningUpdateError(basic({}), { ...basic({}), cexName: 'Binance' })).toBeNull();
    expect(runningUpdateError({ ...basic({}), cexName: 'Binance' }, basic({}))).toBe(
      'A running bot cannot have its exchange removed.',
    );
  });
});
