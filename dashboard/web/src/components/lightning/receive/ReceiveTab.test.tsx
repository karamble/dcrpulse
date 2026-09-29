// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';

const add = vi.fn(async (req: { memo: string; valueAtoms: number }) => ({
  rHashHex: 'aa',
  paymentRequest: 'lnpr',
  valueAtoms: req.valueAtoms,
  amtPaidAtoms: 0,
  creationDate: 1,
  expiry: 3600,
  addIndex: 1,
  status: 'open',
}));
vi.mock('../../../services/lightningApi', () => ({
  addLnInvoice: (req: { memo: string; valueAtoms: number }) => add(req),
  listLnInvoices: async () => ({ invoices: [] }),
  subscribeLnInvoiceEvents: () => () => {},
}));

import { ReceiveTab } from './ReceiveTab';

afterEach(cleanup);

const amountField = () => document.getElementById('ln-rcv-amount') as HTMLInputElement;
const createButton = () => screen.getByRole('button', { name: /Create invoice/ }) as HTMLButtonElement;

// FEMONEY-1: the invoice asks for exactly the amount typed key by key.
it('requests the amount typed key by key', async () => {
  render(<ReceiveTab />);
  const input = amountField();
  for (const ch of '0.05') fireEvent.change(input, { target: { value: input.value + ch } });
  expect(input.value).toBe('0.05');
  fireEvent.click(createButton());
  await waitFor(() => expect(add).toHaveBeenCalled());
  expect(add.mock.calls[0][0].valueAtoms).toBe(5_000_000);
});

it('creates an open-amount invoice when the amount is blank', async () => {
  render(<ReceiveTab />);
  fireEvent.click(createButton());
  await waitFor(() => expect(add).toHaveBeenCalled());
  expect(add.mock.calls[0][0].valueAtoms).toBe(0);
});

it('does not create an invoice from an unfinished amount', () => {
  render(<ReceiveTab />);
  fireEvent.change(amountField(), { target: { value: '.' } });
  expect(createButton().disabled).toBe(true);
});
