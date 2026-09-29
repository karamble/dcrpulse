// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WalletSetup } from './WalletSetup';
import { createNamedWallet, generateSeed, listWallets } from '../services/api';

vi.mock('../services/api', () => ({
  generateSeed: vi.fn(),
  createNamedWallet: vi.fn(),
  importXpub: vi.fn(),
  listWallets: vi.fn(),
}));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const SEED_HEX = 'feedfacecafebeef00seedhexmarker';
const PRIV_PASS = 'privpassmarker42';
const words = Array.from({ length: 33 }, (_, i) => `word${i}`);

describe('wallet create failure', () => {
  it('shows the error without logging the seed or passphrase', async () => {
    const logged = [vi.spyOn(console, 'error'), vi.spyOn(console, 'log'), vi.spyOn(console, 'warn'), vi.spyOn(console, 'debug')];
    logged.forEach((s) => s.mockImplementation(() => {}));
    vi.mocked(listWallets).mockResolvedValue({ wallets: [] } as never);
    vi.mocked(generateSeed).mockResolvedValue({ seedMnemonic: words.join(' '), seedHex: SEED_HEX } as never);
    vi.mocked(createNamedWallet).mockRejectedValue(Object.assign(new Error('Request failed with status code 500'), {
      config: { data: JSON.stringify({ seedHex: SEED_HEX, privatePassphrase: PRIV_PASS }) },
      response: { data: { message: 'wallet already exists' } },
    }));

    render(<WalletSetup />);
    fireEvent.click(await screen.findByRole('button', { name: /Create new wallet/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Create New Wallet' }));
    fireEvent.click(await screen.findByRole('checkbox'));
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    fireEvent.change(screen.getByPlaceholderText('Enter private passphrase (min 8 chars)'), { target: { value: PRIV_PASS } });
    fireEvent.change(screen.getByPlaceholderText('Confirm private passphrase'), { target: { value: PRIV_PASS } });
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    for (const input of screen.getAllByPlaceholderText(/^Enter word #\d+$/)) {
      const n = Number(input.getAttribute('placeholder')!.replace('Enter word #', ''));
      fireEvent.change(input, { target: { value: words[n - 1] } });
    }
    fireEvent.click(screen.getByRole('button', { name: 'Create Wallet' }));

    expect(await screen.findByText('wallet already exists')).toBeTruthy();
    expect(createNamedWallet).toHaveBeenCalledTimes(1);
    const dump = (a: unknown) => (a && typeof a === 'object' ? JSON.stringify({ ...a }) : String(a));
    const out = logged.flatMap((s) => s.mock.calls).flat().map(dump).join('\n');
    expect(out).not.toContain(SEED_HEX);
    expect(out).not.toContain(PRIV_PASS);
  });
});
