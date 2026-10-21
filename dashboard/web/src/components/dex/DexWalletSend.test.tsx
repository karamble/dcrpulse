import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DexWalletState } from '../../services/dcrdexApi';
import * as api from '../../services/dcrdexApi';
import { DexWalletSend } from './DexWalletSend';

vi.mock('../../services/dcrdexApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/dcrdexApi')>()),
  sendDexWallet: vi.fn(),
  estimateDexSendFee: vi.fn(),
}));

const withdrawer = 1 << 6;
const wallet = (assetID: number, symbol: string, traits: number) =>
  ({ assetID, symbol, traits, available: 0.5 }) as DexWalletState;
const addr = 'bc1qexampleaddress';

const fillMax = (w: DexWalletState) => {
  const view = render(<DexWalletSend wallet={w} asset={null} onSent={() => {}} />);
  fireEvent.change(screen.getAllByRole('textbox')[0], { target: { value: addr } });
  fireEvent.click(screen.getByText(/^max /, { selector: 'button' }));
  return view;
};
const confirm = async () => {
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  fireEvent.change(screen.getByPlaceholderText('DCRDEX app password'), { target: { value: 'pw' } });
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
  await waitFor(() => expect(api.sendDexWallet).toHaveBeenCalledTimes(1));
};

beforeEach(() => {
  vi.mocked(api.sendDexWallet).mockReset().mockResolvedValue('coin');
  vi.mocked(api.estimateDexSendFee).mockReset().mockResolvedValue({ fee: 0.0001, feeSymbol: 'BTC', validAddress: true });
});
afterEach(cleanup);

describe('DexWalletSend subtract fees', () => {
  it('max on a withdrawer wallet sends the whole balance with the fee taken out', async () => {
    fillMax(wallet(0, 'BTC', withdrawer));
    const box = screen.getByRole('checkbox', { name: 'Subtract fees from amount sent.' }) as HTMLInputElement;
    expect(box.checked).toBe(true);
    await waitFor(() => expect(api.estimateDexSendFee).toHaveBeenLastCalledWith(0, 0.5, addr, true));
    await confirm();
    expect(api.sendDexWallet).toHaveBeenCalledWith(0, 0.5, addr, 'pw', true);
  });

  it('sends without subtract once the box is cleared', async () => {
    fillMax(wallet(0, 'BTC', withdrawer));
    fireEvent.click(screen.getByRole('checkbox'));
    await confirm();
    expect(api.sendDexWallet).toHaveBeenCalledWith(0, 0.5, addr, 'pw', false);
  });

  it('offers no subtract on a wallet that cannot withdraw', async () => {
    fillMax(wallet(60, 'ETH', 0));
    expect(screen.queryByRole('checkbox')).toBeNull();
    await confirm();
    expect(api.sendDexWallet).toHaveBeenCalledWith(60, 0.5, addr, 'pw', false);
  });

  it('drops a ticked box when the form switches to a wallet that cannot withdraw', async () => {
    const { rerender } = fillMax(wallet(0, 'BTC', withdrawer));
    rerender(<DexWalletSend wallet={wallet(60, 'ETH', 0)} asset={null} onSent={() => {}} />);
    await confirm();
    expect(api.sendDexWallet).toHaveBeenCalledWith(60, 0.5, addr, 'pw', false);
  });
});
