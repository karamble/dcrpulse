import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { LightningDecodedPayReq } from '../../../services/lightningApi';
import * as api from '../../../services/lightningApi';
import { SendTab } from './SendTab';

vi.mock('../../../services/lightningApi', async (original) => ({
  ...(await original<typeof import('../../../services/lightningApi')>()),
  decodeLnPayReq: vi.fn(),
  listLnPayments: vi.fn(async () => ({ payments: [] })),
  streamLnPayment: vi.fn(() => () => {}),
}));

const invoice = (hash: string, numAtoms: number): LightningDecodedPayReq => ({
  destination: '02' + 'ab'.repeat(32),
  paymentHash: hash,
  numAtoms,
  timestamp: Math.floor(Date.now() / 1000),
  expiry: 3600,
  description: hash,
  cltvExpiry: 80,
});

// decodes lets each test resolve a decode when it chooses.
const decodes = () => {
  const pending = new Map<string, (r: LightningDecodedPayReq) => void>();
  vi.mocked(api.decodeLnPayReq).mockImplementation(
    (req: string) => new Promise((resolve) => pending.set(req, resolve)),
  );
  return async (req: string, r: LightningDecodedPayReq) => {
    await waitFor(() => expect(pending.has(req)).toBe(true));
    await act(async () => pending.get(req)!(r));
  };
};

const field = () => screen.getByLabelText('Lightning payment request');
const send = () => screen.getByRole('button', { name: /Send Payment/ }) as HTMLButtonElement;

afterEach(() => {
  cleanup();
  vi.mocked(api.streamLnPayment).mockClear();
});

describe('SendTab', () => {
  it('pays only the invoice whose details are shown', async () => {
    const resolve = decodes();
    render(<SendTab />);

    fireEvent.change(field(), { target: { value: 'lnA' } });
    await resolve('lnA', invoice('hashA', 1000));
    await waitFor(() => expect(send().disabled).toBe(false));

    fireEvent.change(field(), { target: { value: 'lnB' } });
    expect(send().disabled).toBe(true);
    expect(screen.queryByText('hashA')).toBeNull();

    await resolve('lnB', invoice('hashB', 50_000));
    await waitFor(() => expect(send().disabled).toBe(false));
    fireEvent.click(send());
    expect(api.streamLnPayment).toHaveBeenCalledOnce();
    const [req] = vi.mocked(api.streamLnPayment).mock.calls[0];
    expect(req.payReq).toBe('lnB');
    // The backend applies dcrlnd's own default fee limit.
    expect(req).not.toHaveProperty('feeLimitAtoms');
  });

  // FEMONEY-1: an amount typed key by key must keep every character.
  it.each([
    ['0.001', 100_000],
    ['1.5', 150_000_000],
    ['0.00000001', 1],
  ])('pays %s DCR typed key by key into an invoice without an amount', async (typed, atoms) => {
    const resolve = decodes();
    render(<SendTab />);
    fireEvent.change(field(), { target: { value: 'lnOpen' } });
    await resolve('lnOpen', invoice('hashOpen', 0));
    const amount = (await screen.findByPlaceholderText('0.00000000')) as HTMLInputElement;
    for (const ch of typed) fireEvent.change(amount, { target: { value: amount.value + ch } });

    expect(amount.value).toBe(typed);
    fireEvent.click(send());
    const [req] = vi.mocked(api.streamLnPayment).mock.calls[0];
    expect(req.amt).toBe(atoms);
  });

  // LNDEX-5: an amount belongs to the invoice it was typed for.
  it('starts a new invoice without an amount', async () => {
    const resolve = decodes();
    render(<SendTab />);
    fireEvent.change(field(), { target: { value: 'lnA' } });
    await resolve('lnA', invoice('hashA', 0));
    fireEvent.change(await screen.findByPlaceholderText('0.00000000'), { target: { value: '5' } });
    await waitFor(() => expect(send().disabled).toBe(false));

    fireEvent.change(field(), { target: { value: 'lnB' } });
    await resolve('lnB', invoice('hashB', 0));
    expect(((await screen.findByPlaceholderText('0.00000000')) as HTMLInputElement).value).toBe('');
    expect(send().disabled).toBe(true);
  });

  it('ignores a decode that finishes after the invoice changed', async () => {
    const resolve = decodes();
    render(<SendTab />);

    fireEvent.change(field(), { target: { value: 'lnA' } });
    await waitFor(() => expect(api.decodeLnPayReq).toHaveBeenCalledWith('lnA'));
    fireEvent.change(field(), { target: { value: 'lnB' } });
    await resolve('lnA', invoice('hashA', 1000));
    expect(send().disabled).toBe(true);
    fireEvent.click(send());
    expect(api.streamLnPayment).not.toHaveBeenCalled();
  });
});
