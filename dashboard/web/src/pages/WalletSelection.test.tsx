import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { WalletSelection } from './WalletSelection';
import { listWallets, type WalletInfo } from '../services/api';

vi.mock('../services/api', () => ({ listWallets: vi.fn(), selectWallet: vi.fn(), deleteWallet: vi.fn() }));
vi.mock('../components/WalletSetup', () => ({ WalletSetup: () => null }));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const wallet = (over: Partial<WalletInfo>): WalletInfo => ({
  name: 'alice', network: 'mainnet', hasDb: true, isDefault: false, isWatchOnly: false, isPrivacy: false,
  active: false, hasLightning: false, hasBisonRelay: false, hasDex: false, ...over,
});

const openDelete = async (w: WalletInfo) => {
  vi.mocked(listWallets).mockResolvedValue({ wallets: [w] } as never);
  render(<MemoryRouter><WalletSelection /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
  fireEvent.click(screen.getByTitle('Delete'));
};

describe('wallet delete confirmation', () => {
  it('names every kind of data the delete removes', async () => {
    await openDelete(wallet({ hasLightning: true, hasBisonRelay: true, hasDex: true }));
    expect(screen.getByText(/Make sure you have its seed phrase/)).toBeTruthy();
    expect(screen.getByText(/Its Lightning node\. Close its channels first/)).toBeTruthy();
    expect(screen.getByText(/Its Bison Relay identity\./)).toBeTruthy();
    expect(screen.getByText(/Export the DEX app seed first/)).toBeTruthy();
  });

  it('lists only the data that exists', async () => {
    await openDelete(wallet({ hasBisonRelay: true }));
    expect(screen.getByText(/Its Bison Relay identity\./)).toBeTruthy();
    expect(screen.queryByText(/Its Lightning node/)).toBeNull();
    expect(screen.queryByText(/DCRDEX profile/)).toBeNull();
  });

  it('offers no rename', async () => {
    await openDelete(wallet({}));
    expect(screen.queryByTitle('Rename')).toBeNull();
  });
});
