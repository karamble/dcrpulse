// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { DexSettingsPanel } from './DexSettingsPanel';
import { getDexRateSources, saveDexRateSources } from '../../services/dcrdexApi';

vi.mock('../../services/dcrdexApi', () => ({
  getDexStatus: vi.fn(async () => ({ seedBackedUp: true })),
  getDexRateSources: vi.fn(),
  saveDexRateSources: vi.fn(),
  exportDexSeed: vi.fn(),
  markDexSeedBackedUp: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const paprika = 'Coinpaprika (api.coinpaprika.com)';
const dcrdata = 'dcrdata (explorer.dcrdata.org)';

const renderWith = async (exchangeRates: boolean) => {
  vi.mocked(getDexRateSources).mockResolvedValue({
    exchangeRates,
    sources: [
      { name: 'Coinpaprika', enabled: true },
      { name: 'dcrdata', enabled: true },
    ],
  });
  await act(async () => {
    render(<DexSettingsPanel />);
  });
};

it('offers the two price sources and not Messari', async () => {
  await renderWith(true);
  expect((screen.getByLabelText(paprika) as HTMLInputElement).checked).toBe(true);
  expect((screen.getByLabelText(dcrdata) as HTMLInputElement).disabled).toBe(false);
  expect(screen.queryByText(/messari/i)).toBeNull();
});

it('locks the price sources while exchange rates are off', async () => {
  await renderWith(false);
  expect((screen.getByLabelText(paprika) as HTMLInputElement).disabled).toBe(true);
  expect((screen.getByLabelText(dcrdata) as HTMLInputElement).disabled).toBe(true);
  expect(screen.getByText(/Exchange rates are off in Settings > Privacy/)).toBeTruthy();
});

it('saves only the switched source and says when DCRDEX applies it later', async () => {
  await renderWith(true);
  vi.mocked(saveDexRateSources).mockResolvedValue({ applied: false });
  await act(async () => {
    fireEvent.click(screen.getByLabelText(dcrdata));
  });
  expect(saveDexRateSources).toHaveBeenCalledTimes(1);
  expect(saveDexRateSources).toHaveBeenCalledWith({ dcrdata: false });
  expect((screen.getByLabelText(dcrdata) as HTMLInputElement).checked).toBe(false);
  expect((screen.getByLabelText(paprika) as HTMLInputElement).checked).toBe(true);
  expect(screen.getByText('Saved. DCRDEX picks this up the next time it is unlocked.')).toBeTruthy();
});
