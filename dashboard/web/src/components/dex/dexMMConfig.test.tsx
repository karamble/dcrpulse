import { describe, expect, it } from 'vitest';
import {
  applyQuick,
  defaultDraft,
  defaultQuick,
  isQuickShaped,
  lotsPerLevelIncrement,
  quickFromDraft,
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
