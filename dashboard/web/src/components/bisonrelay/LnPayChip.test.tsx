import { StrictMode } from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LnPayChip } from './LnPayChip';
import { getBisonrelayRates } from '../../services/bisonrelayApi';
import { decodeLnPayReq, streamLnPayment, type LightningDecodedPayReq } from '../../services/lightningApi';

vi.mock('../../services/bisonrelayApi', () => ({ getBisonrelayRates: vi.fn() }));
vi.mock('../../services/lightningApi', async (original) => ({
  ...await original<typeof import('../../services/lightningApi')>(),
  decodeLnPayReq: vi.fn(),
  streamLnPayment: vi.fn(),
}));

const A = 'lninvoice-a';
const B = 'lninvoice-b';
const details = (invoice: string, numAtoms = 100_000_000): LightningDecodedPayReq => ({
  destination: 'destination', paymentHash: invoice, numAtoms, timestamp: 1,
  expiry: 3600, description: invoice, cltvExpiry: 40,
});
const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
};
const openConfirmation = async () => fireEvent.click(await screen.findByRole('button', { name: /^Pay \d/ }));
const confirm = () => fireEvent.click(screen.getByRole('button', { name: 'Pay now' }));
const stream = (index = 0) => vi.mocked(streamLnPayment).mock.calls[index];
const settle = (index = 0, status: 'confirmed' | 'failed' = 'confirmed') => act(() => {
  stream(index)[1]({ paymentHash: stream(index)[0].payReq, valueAtoms: 100_000_000,
    feeAtoms: 0, creationDate: 1, status, failureReason: status === 'failed' ? 'No route' : undefined });
});

beforeEach(() => {
  vi.mocked(decodeLnPayReq).mockReset().mockImplementation(async (invoice) => details(invoice));
  vi.mocked(getBisonrelayRates).mockReset().mockResolvedValue({ dcr_usd: 20 } as Awaited<ReturnType<typeof getBisonrelayRates>>);
  vi.mocked(streamLnPayment).mockReset().mockImplementation(() => vi.fn());
});
afterEach(cleanup);

describe('LnPayChip invoice approval', () => {
  it('decodes, cancels, and pays the explicitly confirmed invoice once', async () => {
    render(<StrictMode><LnPayChip invoice={A} /></StrictMode>);
    await openConfirmation();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(streamLnPayment).not.toHaveBeenCalled();
    await openConfirmation();
    confirm();
    expect(streamLnPayment).toHaveBeenCalledTimes(1);
    expect(stream()[0]).toEqual({ payReq: A, feeLimitAtoms: 5_000_000 });
    settle();
    expect(screen.getByText(/^Paid 1 DCR/)).toBeTruthy();
  });

  it('blocks duplicate confirmation clicks before React commits the paying state', async () => {
    render(<LnPayChip invoice={A} />);
    await openConfirmation();
    const button = screen.getByRole('button', { name: 'Pay now' });
    act(() => { button.click(); button.click(); });
    expect(streamLnPayment).toHaveBeenCalledTimes(1);
  });

  it('invalidates an open confirmation before a replacement invoice decodes', async () => {
    const next = deferred<LightningDecodedPayReq>();
    vi.mocked(decodeLnPayReq).mockImplementation((invoice) => invoice === B ? next.promise : Promise.resolve(details(A)));
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    view.rerender(<LnPayChip invoice={B} />);
    const staleConfirmation = screen.queryByRole('button', { name: 'Pay now' });
    if (staleConfirmation) fireEvent.click(staleConfirmation);
    expect(streamLnPayment).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
    expect((screen.getByRole('button', { name: 'Pay Lightning invoice' }) as HTMLButtonElement).disabled).toBe(true);
    expect(streamLnPayment).not.toHaveBeenCalled();
    await act(async () => next.resolve(details(B, 200_000_000)));
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
    await openConfirmation();
    confirm();
    expect(stream()[0]).toEqual({ payReq: B, feeLimitAtoms: 10_000_000 });
  });

  it('requires new approval even when the replacement has the same amount', async () => {
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    view.rerender(<LnPayChip invoice={B} />);
    await screen.findByRole('button', { name: /^Pay 1 DCR/ });
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
    await openConfirmation();
    expect(screen.getByText(B)).toBeTruthy();
    confirm();
    expect(stream()[0].payReq).toBe(B);
  });

  it('preserves unchanged invoice confirmation but resets A to B to A', async () => {
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    view.rerender(<LnPayChip invoice={A} />);
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeTruthy();
    view.rerender(<LnPayChip invoice={B} />);
    view.rerender(<LnPayChip invoice={A} />);
    await screen.findByRole('button', { name: /^Pay 1 DCR/ });
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
  });

  it('ignores an old decode arriving after replacement', async () => {
    const old = deferred<LightningDecodedPayReq>();
    vi.mocked(decodeLnPayReq).mockImplementation((invoice) => invoice === A ? old.promise : Promise.resolve(details(B, 200_000_000)));
    const view = render(<LnPayChip invoice={A} />);
    view.rerender(<LnPayChip invoice={B} />);
    await screen.findByRole('button', { name: /^Pay 2 DCR/ });
    await act(async () => old.resolve(details(A)));
    await openConfirmation();
    expect(screen.getByText(B)).toBeTruthy();
    confirm();
    expect(stream()[0]).toEqual({ payReq: B, feeLimitAtoms: 10_000_000 });
  });

  it('ignores stale exchange rates after invoice replacement', async () => {
    const oldRates = deferred<Awaited<ReturnType<typeof getBisonrelayRates>>>();
    vi.mocked(getBisonrelayRates).mockReturnValueOnce(oldRates.promise);
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    view.rerender(<LnPayChip invoice={B} />);
    await openConfirmation();
    await act(async () => oldRates.resolve({ dcr_usd: 999 } as Awaited<ReturnType<typeof getBisonrelayRates>>));
    expect(screen.queryByText(/999/)).toBeNull();
    confirm();
    expect(stream()[0].payReq).toBe(B);
  });

  it('keeps confirmed and submitted display details fixed when rates arrive late', async () => {
    const rates = deferred<Awaited<ReturnType<typeof getBisonrelayRates>>>();
    vi.mocked(getBisonrelayRates).mockReturnValue(rates.promise);
    render(<LnPayChip invoice={A} />);
    await openConfirmation();
    await act(async () => rates.resolve({ dcr_usd: 999 } as Awaited<ReturnType<typeof getBisonrelayRates>>));
    expect(screen.queryByText(/999/)).toBeNull();
    confirm();
    expect(screen.getByText(/^Paying 1 DCR/).textContent).not.toContain('999');
    settle();
    act(() => stream()[3]());
    expect(screen.getByText(/^Paid 1 DCR/).textContent).not.toContain('999');
    expect(screen.queryByText(/Payment status unknown/)).toBeNull();
  });

  it('does not retain an old confirmation when replacement decoding fails', async () => {
    vi.mocked(decodeLnPayReq).mockImplementation((invoice) => invoice === B ? Promise.reject(new Error('bad invoice')) : Promise.resolve(details(A)));
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    view.rerender(<LnPayChip invoice={B} />);
    await screen.findByText(/Invalid Lightning invoice/);
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
    expect(streamLnPayment).not.toHaveBeenCalled();
    view.rerender(<LnPayChip invoice={A} />);
    await openConfirmation();
    confirm();
    expect(stream()[0].payReq).toBe(A);
  });

  it('retains an active payment across replacement and isolates late stream callbacks', async () => {
    vi.mocked(decodeLnPayReq).mockImplementation(async (invoice) => details(invoice, invoice === B ? 200_000_000 : 100_000_000));
    const closeA = vi.fn();
    const closeB = vi.fn();
    vi.mocked(streamLnPayment).mockReturnValueOnce(closeA).mockReturnValueOnce(closeB);
    const view = render(<LnPayChip invoice={A} />);
    await openConfirmation();
    confirm();
    view.rerender(<LnPayChip invoice={B} />);
    expect(closeA).not.toHaveBeenCalled();
    expect(screen.getByText(/^Paying 1 DCR/)).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
    settle();
    expect(screen.getByText(/^Paid 1 DCR/)).toBeTruthy();
    await openConfirmation();
    confirm();
    expect(stream(1)[0].payReq).toBe(B);
    act(() => { stream(0)[2]('late error'); stream(0)[3](); });
    expect(screen.getByText(/^Paying 2 DCR/)).toBeTruthy();
    view.unmount();
    expect(closeB).toHaveBeenCalledTimes(1);
  });

  it('requires fresh confirmation for a retry after known failure', async () => {
    render(<LnPayChip invoice={A} />);
    await openConfirmation();
    confirm();
    settle(0, 'failed');
    expect(screen.getByText(/No route/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(screen.queryByRole('button', { name: 'Pay now' })).toBeNull();
    await openConfirmation();
    confirm();
    expect(streamLnPayment).toHaveBeenCalledTimes(2);
  });

  it.each(['close', 'error'] as const)('reports an uncertain outcome on premature stream %s without retrying', async (event) => {
    render(<LnPayChip invoice={A} />);
    await openConfirmation();
    confirm();
    act(() => event === 'close' ? stream()[3]() : stream()[2]('connection lost'));
    expect(screen.getByText(/Payment status unknown/)).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
    expect(streamLnPayment).toHaveBeenCalledTimes(1);
  });
});
