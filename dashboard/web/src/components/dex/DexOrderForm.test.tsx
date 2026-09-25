import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DexMarket, DexWalletState, SwapEstimate } from '../../services/dcrdexApi';
import * as api from '../../services/dcrdexApi';
import { DexOrderForm } from './DexOrderForm';

vi.mock('../../services/dcrdexApi', () => ({
  getDexWallets: vi.fn(),
  placeDexOrder: vi.fn(),
  preDexOrder: vi.fn(),
  maxDexBuy: vi.fn(),
  maxDexSell: vi.fn(),
}));
vi.mock('./DexLiveProvider', () => ({ useDexRefreshOnNotes: () => {} }));

const dcrBtc: DexMarket = {
  base: 'DCR', quote: 'BTC', baseID: 42, quoteID: 0, lotSize: 100_000_000, rateStep: 1,
  baseConvFactor: 100_000_000, quoteConvFactor: 100_000_000,
  baseFeeConvFactor: 100_000_000, quoteFeeConvFactor: 100_000_000, baseFeeSymbol: 'DCR', quoteFeeSymbol: 'BTC',
};
const dcrUsdc: DexMarket = {
  base: 'DCR', quote: 'USDC.ETH', baseID: 42, quoteID: 60001, lotSize: 100_000_000, rateStep: 1,
  baseConvFactor: 100_000_000, quoteConvFactor: 1_000_000,
  baseFeeConvFactor: 100_000_000, quoteFeeConvFactor: 1_000_000_000, baseFeeSymbol: 'DCR', quoteFeeSymbol: 'ETH',
};

const wallet = (assetID: number, available: number) => ({ assetID, available }) as DexWalletState;
const swap = (lots: number, value: number): SwapEstimate => ({
  lots, value, maxFees: 0, realisticWorstCase: 0, realisticBestCase: 0, feeReservesPerLot: 0,
});
const estimate = (swapFee: number, redeemFee: number) => ({
  swap: { estimate: { ...swap(0, 0), realisticWorstCase: swapFee } },
  redeem: { estimate: { realisticWorstCase: redeemFee, realisticBestCase: 0 } },
});

const setup = (market: DexMarket, wallets: DexWalletState[]) => {
  vi.mocked(api.getDexWallets).mockResolvedValue(wallets);
  vi.mocked(api.preDexOrder).mockResolvedValue(estimate(0, 0));
  render(<DexOrderForm host="dex.example" market={market} onPlaced={() => {}} />);
};
// Inputs in order: Price, Lots, Amount.
const enter = (price: string, lots: string) => {
  const [p, l] = screen.getAllByRole('spinbutton');
  fireEvent.change(p, { target: { value: price } });
  fireEvent.change(l, { target: { value: lots } });
};
// The side tab carries the same label; the submit button comes last.
const submit = (label: string) => {
  const all = screen.getAllByRole('button', { name: label });
  return all[all.length - 1] as HTMLButtonElement;
};

afterEach(cleanup);

describe('DexOrderForm funding gate', () => {
  it("holds a limit sell to bisonw's max, not to the wallet balance", async () => {
    vi.mocked(api.maxDexSell).mockResolvedValue({ swap: swap(3, 300_000_000), redeem: estimate(0, 0).redeem.estimate });
    setup(dcrBtc, [wallet(42, 10), wallet(0, 1)]);
    fireEvent.click(screen.getAllByRole('button', { name: 'Sell DCR' })[0]);
    enter('0.001', '4');
    await waitFor(() => expect(submit('Sell DCR').disabled).toBe(true));
    expect(screen.getByText('Not enough funds available')).toBeTruthy();

    enter('0.001', '3');
    await waitFor(() => expect(submit('Sell DCR').disabled).toBe(false));
  });

  it("holds a limit buy to bisonw's max lots", async () => {
    vi.mocked(api.maxDexBuy).mockResolvedValue({ swap: swap(2, 0), redeem: estimate(0, 0).redeem.estimate });
    setup(dcrBtc, [wallet(42, 0), wallet(0, 1)]);
    enter('0.001', '3');
    await waitFor(() => expect(submit('Buy DCR').disabled).toBe(true));

    enter('0.001', '2');
    await waitFor(() => expect(submit('Buy DCR').disabled).toBe(false));
  });

  it('does not block on a max estimate that could not be fetched', async () => {
    vi.mocked(api.maxDexSell).mockRejectedValue(new Error('no'));
    setup(dcrBtc, [wallet(42, 10), wallet(0, 1)]);
    fireEvent.click(screen.getAllByRole('button', { name: 'Sell DCR' })[0]);
    enter('0.001', '4');
    await waitFor(() => expect(api.maxDexSell).toHaveBeenCalled());
    await waitFor(() => expect(submit('Sell DCR').disabled).toBe(false));
  });
});

describe('DexOrderForm fee estimate', () => {
  it("shows a token's swap fee in its parent chain's asset", async () => {
    vi.mocked(api.maxDexBuy).mockResolvedValue({ swap: swap(10, 0), redeem: estimate(0, 0).redeem.estimate });
    setup(dcrUsdc, [wallet(42, 0), wallet(60001, 1000)]);
    vi.mocked(api.preDexOrder).mockResolvedValue(estimate(2_000_000, 10_000));
    enter('1', '1');
    await waitFor(() => expect(api.preDexOrder).toHaveBeenCalled());
    await waitFor(() => expect(submit('Buy DCR').disabled).toBe(false));
    fireEvent.click(submit('Buy DCR'));
    expect(await screen.findByText('~0.002 ETH')).toBeTruthy();
    expect(screen.getByText('~0.0001 DCR')).toBeTruthy();
  });
});
