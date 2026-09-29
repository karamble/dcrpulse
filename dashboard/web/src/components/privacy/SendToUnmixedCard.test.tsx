// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const pending: { args: any; resolve: (r: any) => void }[] = [];
const sign = vi.fn(async (..._a: any[]) => ({ txHash: 'ff'.repeat(32) }));

vi.mock('../../services/api', () => ({
  getAccounts: async () => [
    { accountName: 'default', accountNumber: 0, spendableBalance: 10, totalBalance: 10, sharedWallet: false },
  ],
  getNextAddress: async () => ({ address: 'DsUNMIXED', accountNumber: 5 }),
  constructTransaction: (args: any) => new Promise((resolve) => pending.push({ args, resolve })),
  signPublishTransaction: (...a: any[]) => sign(...a),
}));

import { SendToUnmixedCard } from './SendToUnmixedCard';

// built pays `payment` atoms; the rest of the outputs is change.
const built = (payment: number) => ({
  unsignedTxHex: `HEX_${payment}`,
  inputsTotalAtoms: 900000000,
  outputsTotalAtoms: 899999000,
  changeAtoms: 899999000 - payment,
  feeAtoms: 1000,
  totalDebitedAtoms: payment + 1000,
  estimatedSignedSize: 300,
});

beforeEach(() => {
  pending.length = 0;
});
afterEach(cleanup);

const sendButton = () => screen.getByRole('button', { name: /Send to unmixed/ }) as HTMLButtonElement;

it('does not offer a build for an amount other than the one typed', async () => {
  render(<SendToUnmixedCard changeAccount={5} />);
  const amount = await screen.findByPlaceholderText('0.00000000');
  fireEvent.change(amount, { target: { value: '1' } });
  await waitFor(() => expect(pending.length).toBe(1), { timeout: 4000 });
  await act(async () => pending[0].resolve(built(100000000)));
  expect(sendButton().disabled).toBe(false);

  fireEvent.change(amount, { target: { value: '5' } });
  expect(sendButton().disabled).toBe(true);
});

// FEMONEY-3: the older build landing last must not replace the newer one.
it('signs the build for the latest amount and shows that amount', async () => {
  render(<SendToUnmixedCard changeAccount={5} />);
  const amount = await screen.findByPlaceholderText('0.00000000');
  fireEvent.change(amount, { target: { value: '1' } });
  await waitFor(() => expect(pending.length).toBe(1), { timeout: 4000 });
  fireEvent.change(amount, { target: { value: '5' } });
  await waitFor(() => expect(pending.length).toBe(2), { timeout: 4000 });
  await act(async () => pending[1].resolve(built(500000000)));
  await act(async () => pending[0].resolve(built(100000000)));

  fireEvent.click(sendButton());
  expect(screen.queryByText('5.00000000 DCR')).not.toBeNull();
  fireEvent.change(document.getElementById('send-passphrase') as HTMLInputElement, { target: { value: 'pw' } });
  fireEvent.click(screen.getByRole('button', { name: /Confirm & Send/ }));
  await waitFor(() => expect(sign).toHaveBeenCalled());
  expect(sign.mock.calls[0][1]).toBe('HEX_500000000');
});
