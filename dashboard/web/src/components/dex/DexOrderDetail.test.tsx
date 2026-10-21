import { act, cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DexFullMatch, DexOrder, DexOrderFull } from '../../services/dcrdexApi';
import * as api from '../../services/dcrdexApi';
import { DexOrderDetail } from './DexOrderDetail';

vi.mock('../../services/dcrdexApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/dcrdexApi')>()),
  getDexOrder: vi.fn(),
  getDexWallets: vi.fn(),
}));
let refresh = () => {};
vi.mock('./DexLiveProvider', () => ({
  useDexRefreshOnNotes: (_: string[], fn: () => void) => {
    refresh = fn;
  },
}));
vi.mock('../../hooks/useVisiblePoll', () => ({ startVisiblePoll: () => () => {} }));

const order: DexOrder = {
  id: 'ab12', host: 'dex.example', marketName: 'dcr_btc', baseID: 42, quoteID: 0,
  type: 'limit', sell: true, status: 'executed', stamp: 0, submitTime: 0,
  quantity: 300_000_000, filled: 300_000_000, settled: 0, rate: 1_000_000,
};
const match = (matchID: string, side: string, stamp: number, qty: number): DexFullMatch => ({
  matchID, side, stamp, qty, status: 'MakerSwapCast', revoked: false, rate: 1_000_000, feeRate: 10, isCancel: false,
});
const full = (matches: DexFullMatch[]): DexOrderFull => ({ ...order, matches });

// Each card header reads role, then "<qty> DCR @ <rate> BTC"; keep role and qty.
const cards = () =>
  screen
    .getAllByText(/^(Maker|Taker)$/)
    .map((el) => `${el.textContent} ${el.nextElementSibling?.textContent?.split(' @')[0]}`);

const show = async (first: DexFullMatch[]) => {
  vi.mocked(api.getDexWallets).mockResolvedValue([]);
  vi.mocked(api.getDexOrder).mockResolvedValue(full(first));
  render(
    <MemoryRouter>
      <DexOrderDetail order={order} onBack={() => {}} onCancel={() => {}} />
    </MemoryRouter>,
  );
  await screen.findByText('Matches (2)');
};
const refetch = async (next: DexFullMatch[]) => {
  vi.mocked(api.getDexOrder).mockResolvedValue(full(next));
  await act(async () => refresh());
};

afterEach(cleanup);

describe('DexOrderDetail match order', () => {
  it('keeps the older match first however bisonw orders them', async () => {
    const maker = match('11', 'Maker', 1_000, 100_000_000);
    const taker = match('22', 'Taker', 2_000, 200_000_000);
    await show([maker, taker]);
    expect(cards()).toEqual(['Maker 1 DCR', 'Taker 2 DCR']);
    await refetch([taker, maker]);
    expect(cards()).toEqual(['Maker 1 DCR', 'Taker 2 DCR']);
  });

  it('holds matches from the same epoch in place', async () => {
    const a = match('aa', 'Taker', 5_000, 100_000_000);
    const b = match('bb', 'Maker', 5_000, 200_000_000);
    await show([a, b]);
    expect(cards()).toEqual(['Taker 1 DCR', 'Maker 2 DCR']);
    await refetch([b, a]);
    expect(cards()).toEqual(['Taker 1 DCR', 'Maker 2 DCR']);
  });
});
