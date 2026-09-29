// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const pending: { args: any; resolve: (r: any) => void }[] = [];

vi.mock('../../services/api', () => ({
  getAccounts: async () => [
    { accountName: 'default', accountNumber: 0, spendableBalance: 10, totalBalance: 10, sharedWallet: false },
  ],
  getPrivacyStatus: async () => null,
  getAutobuyerStatus: async () => null,
  getPurchaseStatus: async () => null,
  getNextAddress: async () => ({ address: 'x', accountNumber: 0 }),
  validateAddress: async (a: string) => ({ isValid: a === 'DsAAA', isMine: false, accountNumber: 0 }),
  constructTransaction: (args: any) => new Promise((resolve) => pending.push({ args, resolve })),
  signPublishTransaction: async () => ({ txHash: 'ff'.repeat(32) }),
}));
vi.mock('../../hooks/useVisiblePoll', () => ({ useVisiblePoll: () => {} }));

import { SendTab } from './SendTab';

const built = (hex: string) => ({
  unsignedTxHex: hex,
  inputsTotalAtoms: 200000000,
  outputsTotalAtoms: 199999000,
  changeAtoms: 99999000,
  feeAtoms: 1000,
  totalDebitedAtoms: 100001000,
  estimatedSignedSize: 300,
});

beforeEach(() => {
  pending.length = 0;
});
afterEach(cleanup);

const fillForm = async (address: string) => {
  render(
    <MemoryRouter>
      <SendTab />
    </MemoryRouter>,
  );
  const recipient = await screen.findByPlaceholderText(/DsXXXX/);
  fireEvent.change(recipient, { target: { value: address } });
  fireEvent.change(screen.getByPlaceholderText('0.00000000'), { target: { value: '1' } });
  await waitFor(() => expect(pending.length).toBe(1), { timeout: 4000 });
  return recipient;
};

const sendButton = () => screen.getByRole('button', { name: /^Send$/ }) as HTMLButtonElement;

it('offers a transaction built for the form as it is', async () => {
  await fillForm('DsAAA');
  await act(async () => pending[0].resolve(built('HEX_FOR_DsAAA')));
  expect(screen.queryByText('Total debited')).not.toBeNull();
  expect(sendButton().disabled).toBe(false);
});

// FEMONEY-2: a build that lands after the recipient changed must not be signed.
it('drops a build that lands after the recipient changed', async () => {
  const recipient = await fillForm('DsAAA');
  fireEvent.change(recipient, { target: { value: 'DsBBB' } });
  await waitFor(() => expect(screen.queryByText(/Invalid address for this network/)).not.toBeNull(), {
    timeout: 4000,
  });
  await act(async () => pending[0].resolve(built('HEX_FOR_DsAAA')));
  expect(screen.queryByText('Total debited')).toBeNull();
  expect(sendButton().disabled).toBe(true);
});
