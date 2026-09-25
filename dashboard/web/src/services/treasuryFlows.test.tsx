import { describe, expect, it, vi } from 'vitest';

// Months are UTC whatever the viewer's zone; run in one where they differ.
vi.stubEnv('TZ', 'America/Sao_Paulo');
import type { TAddRecord, TSpendRecord } from './treasuryStorage';
import { flowsByMonth, monthlyRows, runway, yearlyRows } from './treasuryFlows';

const spend = (timestamp: string, amountAtoms: number, feeAtoms = 0): TSpendRecord => ({
  txHash: `${timestamp}-${amountAtoms}`, amountAtoms, feeAtoms, payee: 'Ds', blockHeight: 1, timestamp, voteResult: 'approved',
});
const add = (timestamp: string, amountAtoms: number): TAddRecord => ({ txHash: timestamp, amountAtoms, blockHeight: 1, timestamp });

describe('flowsByMonth', () => {
  it('puts every record in its UTC month and counts spend fees', () => {
    const m = flowsByMonth(
      [spend('2026-05-17T06:02:09+02:00', 606308751309, 14020), spend('2026-05-31T23:30:00-02:00', 100)],
      [add('2026-05-02T00:00:00Z', 250000000000)],
      { '2026-05': 470000000000, '2026-06': 460000000000 },
    );
    expect(m.get('2026-05')).toEqual({ blockRewardAtoms: 470000000000, contributionsAtoms: 250000000000, spendsAtoms: 606308765329 });
    expect(m.get('2026-06')).toEqual({ blockRewardAtoms: 460000000000, contributionsAtoms: 0, spendsAtoms: 100 });
  });
});

describe('monthlyRows and yearlyRows', () => {
  const m = flowsByMonth([spend('2024-03-10T00:00:00Z', 7)], [], { '2024-03': 5, '2026-02': 3 });
  const now = new Date('2026-04-15T12:00:00Z');

  it('lists a year month by month, up to the current month', () => {
    const rows = monthlyRows(m, 2026, now);
    expect(rows.map((r) => r.label)).toEqual(['Jan', 'Feb', 'Mar', 'Apr']);
    expect(rows[1]).toEqual({ label: 'Feb', blockRewardAtoms: 3, contributionsAtoms: 0, spendsAtoms: 0 });
    expect(monthlyRows(m, 2024, now)).toHaveLength(12);
  });

  it('lists every year from the first record to now, empty ones included', () => {
    expect(yearlyRows(m, now)).toEqual([
      { label: '2024', blockRewardAtoms: 5, contributionsAtoms: 0, spendsAtoms: 7 },
      { label: '2025', blockRewardAtoms: 0, contributionsAtoms: 0, spendsAtoms: 0 },
      { label: '2026', blockRewardAtoms: 3, contributionsAtoms: 0, spendsAtoms: 0 },
    ]);
  });
});

describe('runway', () => {
  const now = new Date('2026-09-25T10:00:00Z');

  it('spreads the last twelve full months of spending over the balance', () => {
    const r = runway(
      88_000_000_000_000,
      [
        spend('2025-09-01T00:00:00Z', 1_000_000_000_000, 10_000), // first second of the window
        spend('2026-08-31T23:59:59Z', 200_000_000_000, 20_000), // last second
        spend('2025-08-31T23:59:59Z', 9_000_000_000_000), // before it
        spend('2026-09-02T00:00:00Z', 9_000_000_000_000), // the current month
      ],
      now,
    );
    expect(r.outflowAtoms).toBe(1_200_000_030_000);
    expect(r.monthlyAtoms).toBe(100_000_002_500);
    expect(r.months).toBeCloseTo(879.9999780000005, 6);
  });

  it('has no runway when nothing was spent', () => {
    expect(runway(5, [], now).months).toBeNull();
  });
});
