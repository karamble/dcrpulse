// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { TAddRecord, TSpendRecord } from './treasuryStorage';

// Everything below is in atoms and counts what the scan recorded; nothing is
// inferred from the balance.

export interface Flow {
  blockRewardAtoms: number;
  contributionsAtoms: number;
  spendsAtoms: number; // Paid outputs plus the fees the treasury paid
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

const utcMonth = (iso: string): string => {
  const d = new Date(iso);
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
};

const zero = (): Flow => ({ blockRewardAtoms: 0, contributionsAtoms: 0, spendsAtoms: 0 });

// flowsByMonth groups the records by UTC month ("2026-05").
export const flowsByMonth = (
  tspends: TSpendRecord[],
  tadds: TAddRecord[],
  tbaseByMonth: Record<string, number>,
): Map<string, Flow> => {
  const out = new Map<string, Flow>();
  const at = (month: string) => {
    let f = out.get(month);
    if (!f) out.set(month, (f = zero()));
    return f;
  };
  for (const [month, atoms] of Object.entries(tbaseByMonth)) at(month).blockRewardAtoms += atoms;
  for (const t of tadds) at(utcMonth(t.timestamp)).contributionsAtoms += t.amountAtoms;
  for (const t of tspends) at(utcMonth(t.timestamp)).spendsAtoms += t.amountAtoms + t.feeAtoms;
  return out;
};

// monthlyRows is one year's months; the current year stops at the current month.
export const monthlyRows = (byMonth: Map<string, Flow>, year: number, now: Date) => {
  const last = year === now.getUTCFullYear() ? now.getUTCMonth() : 11;
  const rows: (Flow & { label: string })[] = [];
  for (let m = 0; m <= last; m++) {
    rows.push({ label: MONTHS[m], ...(byMonth.get(`${year}-${String(m + 1).padStart(2, '0')}`) ?? zero()) });
  }
  return rows;
};

// yearlyRows is every year from the first with any flow through now.
export const yearlyRows = (byMonth: Map<string, Flow>, now: Date) => {
  const years = [...byMonth.keys()].map((k) => Number(k.slice(0, 4)));
  if (years.length === 0) return [];
  const rows: (Flow & { label: string })[] = [];
  for (let y = Math.min(...years); y <= now.getUTCFullYear(); y++) {
    const f = zero();
    for (let m = 1; m <= 12; m++) {
      const v = byMonth.get(`${y}-${String(m).padStart(2, '0')}`);
      if (!v) continue;
      f.blockRewardAtoms += v.blockRewardAtoms;
      f.contributionsAtoms += v.contributionsAtoms;
      f.spendsAtoms += v.spendsAtoms;
    }
    rows.push({ label: String(y), ...f });
  }
  return rows;
};

// runway spreads the last twelve complete UTC months of spending over the
// balance. months is null when nothing was spent in that time.
export const runway = (balanceAtoms: number, tspends: TSpendRecord[], now: Date) => {
  const end = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1);
  const start = Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - 12, 1);
  let outflowAtoms = 0;
  for (const t of tspends) {
    const at = new Date(t.timestamp).getTime();
    if (at >= start && at < end) outflowAtoms += t.amountAtoms + t.feeAtoms;
  }
  return {
    outflowAtoms,
    monthlyAtoms: outflowAtoms / 12,
    months: outflowAtoms > 0 ? balanceAtoms / (outflowAtoms / 12) : null,
  };
};
