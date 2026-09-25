import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { TreasuryScanResults } from './treasuryApi';
import shipped from '../../public/tspend-snapshot.json';
import {
  applyScanResults,
  clearTreasuryData,
  exportTreasuryData,
  getLastSyncHeight,
  getTBaseByMonth,
  getAllTSpends,
  importTreasuryData,
  syncWithSnapshot,
} from './treasuryStorage';

const results = (fromHeight: number, toHeight: number, tbase: Record<string, number>, txHash?: string): TreasuryScanResults => ({
  fromHeight,
  toHeight,
  tadds: [],
  tbaseByMonth: tbase,
  tspends: txHash
    ? [{ txHash, amount: 1, amountAtoms: 100000000, feeAtoms: 5, payee: 'Ds', blockHeight: toHeight, blockHash: 'h', timestamp: '2021-05-30T00:00:00Z', voteResult: 'approved' }]
    : [],
});

// jsdom here has no localStorage; a map stands in for it.
beforeEach(() => {
  const items = new Map<string, string>();
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => items.get(k) ?? null,
    setItem: (k: string, v: string) => void items.set(k, v),
    removeItem: (k: string) => void items.delete(k),
    clear: () => items.clear(),
  });
});
afterEach(() => vi.unstubAllGlobals());

describe('applyScanResults', () => {
  it('takes results that continue exactly where the stored data ends, once', () => {
    expect(applyScanResults(results(552449, 552500, { '2021-05': 10 }))).toBe(false); // must start at activation
    expect(applyScanResults(results(552448, 552500, { '2021-05': 10 }, 'a'))).toBe(true);
    expect(applyScanResults(results(552448, 552500, { '2021-05': 10 }, 'a'))).toBe(false); // the same scan again
    expect(applyScanResults(results(552502, 552600, { '2021-05': 10 }))).toBe(false); // a gap
    expect(applyScanResults(results(552501, 552501, { '2021-05': 3, '2021-06': 4 }))).toBe(true);
    expect(getTBaseByMonth()).toEqual({ '2021-05': 13, '2021-06': 4 });
    expect(getLastSyncHeight()).toBe(552501);
    expect(getAllTSpends().map((t) => [t.txHash, t.amountAtoms, t.feeAtoms])).toEqual([['a', 100000000, 5]]);
  });
});

describe('export and import', () => {
  it('round-trips the whole data set and only accepts one that reaches further', () => {
    applyScanResults(results(552448, 552500, { '2021-05': 10 }, 'a'));
    const exported = exportTreasuryData();
    clearTreasuryData();
    expect(importTreasuryData(exported)).toEqual({ success: true });
    expect(exportTreasuryData()).toBe(exported);
    expect(importTreasuryData(exported).success).toBe(false);
    expect(importTreasuryData(JSON.stringify({ version: 1, tspends: [], tadds: [], tbaseByMonth: {}, lastSyncHeight: 999999 })).success).toBe(false);
  });
});

describe('syncWithSnapshot', () => {
  const snapshot = { version: 2, tspends: [], tadds: [], tbaseByMonth: { '2021-05': 7 }, lastSyncHeight: 552600 };
  const serve = (body: unknown) =>
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => body })));

  it('loads a snapshot that reaches further than the stored data', async () => {
    serve(snapshot);
    expect(await syncWithSnapshot()).toEqual({ success: true, synced: true });
    expect(getLastSyncHeight()).toBe(552600);
  });

  it('keeps stored data that already reaches as far', async () => {
    applyScanResults(results(552448, 552700, { '2021-05': 1 }));
    serve(snapshot);
    expect(await syncWithSnapshot()).toEqual({ success: true, synced: false });
    expect(getTBaseByMonth()).toEqual({ '2021-05': 1 });
  });

  it('drops data stored in the old format', async () => {
    localStorage.setItem('decred-pulse-treasury-history', JSON.stringify({ version: 1, tspends: [], tadds: [], tbaseByMonth: {}, lastSyncHeight: 1080000 }));
    serve(snapshot);
    expect(await syncWithSnapshot()).toEqual({ success: true, synced: true });
    expect(getLastSyncHeight()).toBe(552600);
  });

  it('loads the snapshot the app ships', async () => {
    serve(shipped);
    expect(await syncWithSnapshot()).toEqual({ success: true, synced: true });
    expect(getLastSyncHeight()).toBe(shipped.lastSyncHeight);
  });
});
