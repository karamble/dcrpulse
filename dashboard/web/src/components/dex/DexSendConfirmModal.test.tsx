import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../services/dcrdexApi';
import { DexSendConfirmModal } from './DexSendConfirmModal';

vi.mock('../../services/dcrdexApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/dcrdexApi')>()),
  sendDexWallet: vi.fn(),
  estimateDexSendFee: vi.fn(),
}));

const addr = 'bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq';
const onClose = vi.fn();
const onSent = vi.fn();

const open = (subtract: boolean, symbol = 'BTC') =>
  render(
    <DexSendConfirmModal
      assetID={0}
      symbol={symbol}
      address={addr}
      value={0.5}
      decimals={8}
      subtract={subtract}
      onClose={onClose}
      onSent={onSent}
    />,
  );
const password = () => screen.getByLabelText(/DCRDEX app password/) as HTMLInputElement;
const confirm = () => {
  fireEvent.change(password(), { target: { value: 'pw' } });
  fireEvent.click(screen.getByRole('button', { name: 'Confirm & Send' }));
};

beforeEach(() => {
  vi.mocked(api.sendDexWallet).mockReset().mockResolvedValue('coin1');
  vi.mocked(api.estimateDexSendFee).mockReset().mockResolvedValue({ fee: 0.00012, feeSymbol: 'btc', validAddress: true });
  onClose.mockReset();
  onSent.mockReset();
});
afterEach(cleanup);

describe('DexSendConfirmModal', () => {
  it("shows the full address, the amount and bisonw's fee for this send", async () => {
    open(true);
    expect(screen.getByText(addr)).toBeTruthy();
    expect(screen.getByText('0.5 BTC')).toBeTruthy();
    await screen.findByText('~0.00012 BTC');
    expect(api.estimateDexSendFee).toHaveBeenCalledWith(0, 0.5, addr, true);
    expect(screen.getByText('Taken out of the amount sent.')).toBeTruthy();
  });

  it('says the fee is paid on top when nothing is subtracted', async () => {
    open(false);
    await screen.findByText('~0.00012 BTC');
    expect(api.estimateDexSendFee).toHaveBeenCalledWith(0, 0.5, addr, false);
    expect(screen.getByText('Paid on top of the amount.')).toBeTruthy();
  });

  it('names the parent balance when the fee is paid in another asset', async () => {
    vi.mocked(api.estimateDexSendFee).mockResolvedValue({ fee: 0.001, feeSymbol: 'eth', validAddress: true });
    open(false, 'USDC.ETH');
    expect(await screen.findByText('Paid on top of the amount, from your ETH balance.')).toBeTruthy();
  });

  it('still sends when the fee estimate fails', async () => {
    vi.mocked(api.estimateDexSendFee).mockRejectedValue(new Error('no fee rate'));
    open(true);
    await screen.findByText('Fee estimate not available');
    confirm();
    await waitFor(() => expect(onSent).toHaveBeenCalledWith('coin1'));
    expect(api.sendDexWallet).toHaveBeenCalledWith(0, 0.5, addr, 'pw', true);
  });

  it('stays open with the error and an empty password when the send fails', async () => {
    vi.mocked(api.sendDexWallet).mockRejectedValue(new Error('insufficient funds'));
    open(true);
    confirm();
    await screen.findByText('insufficient funds');
    expect(password().value).toBe('');
    expect(onSent).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});
