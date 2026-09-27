import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { GamingPayout, GamingSpend } from '../../services/gamingApi';
import type { GamingSpendsSnapshot } from '../../hooks/useGamingSpends';
import { GamingApprovalsPanel, waitingOrder } from './GamingApprovalsPanel';
import { PayoutShare } from './GamingPayoutApprovals';
import { approveGamingPayout, decideGamingSpend } from '../../services/gamingApi';

const now = Math.floor(Date.now() / 1000);
let payouts: GamingPayout[] = [];
let feed: GamingSpendsSnapshot;
const sent: string[] = [];

vi.mock('../../hooks/useVisiblePoll', async () => {
  const { useEffect } = await import('react');
  return { useVisiblePoll: (fn: () => void) => useEffect(() => fn(), []) };
});
vi.mock('../../hooks/useGamingSpends', () => ({
  useGamingSpends: () => feed,
  refreshGamingSpends: vi.fn(async () => {}),
}));
vi.mock('../../services/gamingApi', () => ({
  getGamingPayouts: async () => payouts,
  approveGamingPayout: vi.fn(),
  rejectGamingPayout: vi.fn(),
  sendGamingPayoutSignatures: (id: string) => { sent.push(id); return Promise.resolve({}); },
  decideGamingSpend: vi.fn(),
  getGamingSpendHistory: vi.fn(),
}));

const payout = (over: Partial<GamingPayout>): GamingPayout => ({
  id: 'tx1', table: 'table1', scope: { game: 'poker' }, state: 'awaiting_approval',
  payments: [{ key: '02aa', atoms: 199990000 }],
  destinations: { '02aa': 'DsOurs', '02bb': 'DsTheirs' },
  feeAtoms: 10000, expiresAt: now + 600, signatures: {},
  mine: { key: '02aa', address: 'DsOurs', stakeAtoms: 100000000, receiveAtoms: 199990000 },
  ...over,
});

const spend = (over: Partial<GamingSpend>): GamingSpend => ({
  depositId: 'dep', tableId: 'table1', depositKind: 'stake', fundingFeeAtoms: 2000,
  recoveryLockBlocks: 288, id: 's1', game: 'stakewars', address: 'DsBuyIn', amountAtoms: 5000000,
  state: 'pending', requestedAt: now - 10, expiresAt: now + 110, ...over,
});

beforeEach(() => {
  payouts = [];
  sent.length = 0;
  feed = {
    pending: [], publishing: [], decided: [], decidedTotal: 0, usedToday: {},
    serverNow: now, offset: 0, failures: 0, error: null, lastOkAt: Date.now(),
  };
});
afterEach(cleanup);

// order returns the rendered order of the given texts.
const order = (...texts: string[]) => {
  const all = document.body.textContent ?? '';
  return texts.map((t) => all.indexOf(t));
};

describe('Waiting for you', () => {
  it('puts the sooner deadline first across buy-ins and payouts', () => {
    const p = payout({ id: 'pay', expiresAt: now + 60 });
    const s = spend({ id: 'buy', expiresAt: now + 100 });
    expect(waitingOrder([s], [p], now).map((w) => w.id)).toEqual(['pay', 'buy']);
    expect(waitingOrder([spend({ id: 'buy', expiresAt: now + 30 })], [p], now).map((w) => w.id)).toEqual(['buy', 'pay']);
  });

  it('puts payouts without a deadline after timed items, and leaves out payouts needing nothing', () => {
    const items = waitingOrder(
      [spend({ id: 'buy', expiresAt: now + 100 })],
      [
        payout({ id: 'unsent', state: 'awaiting_signatures', signaturesSent: 'unsent', expiresAt: now + 5 }),
        payout({ id: 'collecting', state: 'awaiting_signatures', signaturesSent: 'sent' }),
        payout({ id: 'lapsed', expiresAt: now - 1 }),
        payout({ id: 'done', state: 'confirmed' }),
      ],
      now,
    );
    expect(items.map((w) => w.id)).toEqual(['buy', 'unsent']);
  });

  it('breaks equal deadlines the same way every time: buy-in first, then by id', () => {
    const items = waitingOrder(
      [spend({ id: 'b2', expiresAt: now + 50 }), spend({ id: 'b1', expiresAt: now + 50 })],
      [payout({ id: 'a0', expiresAt: now + 50 })],
      now,
    );
    expect(items.map((w) => w.id)).toEqual(['b1', 'b2', 'a0']);
  });

  it('renders waiting items above buy-ins being paid, and buy-ins above match payouts', async () => {
    feed.pending = [spend({ id: 'buy', amountAtoms: 7000000 })];
    feed.publishing = [spend({ id: 'paying', state: 'publishing', amountAtoms: 3000000 })];
    payouts = [payout({ id: 'done', state: 'confirmed', table: 'finished-table' }), payout({ id: 'pay', table: 'waiting-table', expiresAt: now + 60 })];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    await screen.findByText('Table waiting-table');
    const [head, pay, buy, being, buyins, matches, done] = order('Waiting for you', 'Table waiting-table', '0.07 DCR', 'being paid', 'Buy-in approvals', 'Match payouts', 'Table finished-table');
    expect(head).toBeGreaterThanOrEqual(0);
    expect(head < pay && pay < buy && buy < being).toBe(true);
    expect(buyins < being && being < matches && matches < done).toBe(true);
  });

  it('shows no waiting section when nothing waits', async () => {
    payouts = [payout({ state: 'confirmed' })];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    await screen.findByText('You');
    expect(screen.queryByText('Waiting for you')).toBeNull();
  });
});

describe('Match payouts order', () => {
  it('keeps one order however the ledger returns them, collecting before finished', async () => {
    const a = payout({ id: 'a', table: 'tA', state: 'confirmed', expiresAt: now - 100 });
    const b = payout({ id: 'b', table: 'tB', state: 'awaiting_signatures', signaturesSent: 'sent', expiresAt: now - 200 });
    const c = payout({ id: 'c', table: 'tC', state: 'confirmed', expiresAt: now - 50 });
    payouts = [a, b, c];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    await screen.findByText('Table tA');
    const first = order('Table tB', 'Table tC', 'Table tA');
    cleanup();
    payouts = [c, a, b];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    await screen.findByText('Table tA');
    const second = order('Table tB', 'Table tC', 'Table tA');
    expect(first[0] < first[1] && first[1] < first[2]).toBe(true);
    expect(second[0] < second[1] && second[1] < second[2]).toBe(true);
  });
});

describe('payout cards', () => {
  it("marks the operator's output and states stake, payout and net", async () => {
    payouts = [payout({})];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    expect(await screen.findByText('You')).toBeTruthy();
    expect(screen.getByText('DsOurs').parentElement?.textContent).toContain('You');
    const line = screen.getByText(/You staked/).textContent;
    expect(line).toContain('1 DCR');
    expect(line).toContain('1.9999 DCR');
    expect(line).toContain('net +0.9999 DCR');
  });

  it('offers one send only when Bison Relay never took our signatures', async () => {
    payouts = [payout({ id: 'unsent', state: 'awaiting_signatures', signaturesSent: 'unsent' })];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    expect(await screen.findByText(/did not reach Bison Relay/)).toBeTruthy();
    fireEvent.click(screen.getByText('Send signatures'));
    await waitFor(() => expect(sent).toEqual(['unsent']));
  });

  it('says an unknown outcome plainly', async () => {
    payouts = [payout({ state: 'awaiting_signatures', signaturesSent: 'uncertain' })];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    expect(await screen.findByText(/could not be confirmed/)).toBeTruthy();
  });

  it('shows nothing to send once sent', async () => {
    payouts = [payout({ state: 'awaiting_signatures', signaturesSent: 'sent' })];
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    expect(await screen.findByText('You')).toBeTruthy();
    expect(screen.queryByText('Send signatures')).toBeNull();
  });
});

describe('buy-in decisions', () => {
  it('keeps one passphrase field for the whole panel', async () => {
    feed.pending = [spend({ id: 'one', expiresAt: now + 100 }), spend({ id: 'two', expiresAt: now + 110 })];
    const real = Date.now;
    render(<GamingApprovalsPanel policies={{}} bridgeEnabled gameCount={1} />);
    vi.spyOn(Date, 'now').mockImplementation(() => real() + 5000);
    const [first] = await waitFor(() => {
      const buttons = screen.getAllByText('Approve...') as HTMLButtonElement[];
      if (buttons.some((b) => b.disabled)) throw new Error('still fresh');
      return buttons;
    }, { timeout: 3000 });
    fireEvent.click(first);
    expect(screen.getAllByPlaceholderText('Wallet passphrase')).toHaveLength(1);
    expect((screen.getByText('Approve...') as HTMLButtonElement).disabled).toBe(true);
    vi.restoreAllMocks();
  });
});

describe('PayoutShare', () => {
  it('warns when the payout pays the operator nothing', () => {
    render(<PayoutShare payout={payout({ payments: [{ key: '02bb', atoms: 199990000 }], mine: { key: '02aa', address: 'DsOurs', stakeAtoms: 100000000, receiveAtoms: 0 } })} />);
    expect(screen.getByText(/You staked/).textContent).toContain('net −1 DCR');
    expect(screen.getByRole('alert').textContent).toContain('pays you nothing');
  });

  it("warns when the operator's output cannot be identified", () => {
    render(<PayoutShare payout={payout({ mine: null })} />);
    expect(screen.getByRole('alert').textContent).toContain('could not be identified');
  });
});

describe('the armed buy-in', () => {
  // armA renders two waiting buy-ins, waits out the arrival guard, and opens
  // the passphrase field on the first.
  const armA = async () => {
    const view = render(<GamingApprovalsPanel />);
    const clock = vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 2000);
    view.rerender(<GamingApprovalsPanel />);
    fireEvent.click(screen.getAllByText('Approve...')[0]);
    expect(await screen.findByPlaceholderText('Wallet passphrase')).toBeTruthy();
    expect(screen.getByText('Answering another request.')).toBeTruthy();
    return { view, clock };
  };
  const bApprove = () => { const all = screen.getAllByText('Approve...'); return all[all.length - 1] as HTMLButtonElement; };

  it('frees the other requests when the armed one leaves the list', async () => {
    feed.pending = [spend({ id: 'a1', game: 'stakewars' }), spend({ id: 'b2', game: 'orbitgolf' })];
    const { view, clock } = await armA();
    feed = { ...feed, pending: [spend({ id: 'b2', game: 'orbitgolf' })] };
    view.rerender(<GamingApprovalsPanel />);
    await waitFor(() => expect(screen.queryByText('Answering another request.')).toBeNull());
    expect(screen.queryByPlaceholderText('Wallet passphrase')).toBeNull();
    expect(bApprove().disabled).toBe(false);
    clock.mockRestore();
  });

  it('frees the other requests when the armed one lapses', async () => {
    feed.pending = [spend({ id: 'a1', game: 'stakewars' }), spend({ id: 'b2', game: 'orbitgolf' })];
    const { view, clock } = await armA();
    feed = { ...feed, pending: [spend({ id: 'a1', game: 'stakewars', expiresAt: now - 1 }), spend({ id: 'b2', game: 'orbitgolf' })] };
    view.rerender(<GamingApprovalsPanel />);
    await waitFor(() => expect(screen.queryByPlaceholderText('Wallet passphrase')).toBeNull());
    expect(bApprove().disabled).toBe(false);
    clock.mockRestore();
  });

  it('empties the passphrase field once it is sent, even when the approval fails', async () => {
    vi.mocked(decideGamingSpend).mockRejectedValueOnce(new Error('wrong passphrase'));
    feed.pending = [spend({ id: 'a1', game: 'stakewars' }), spend({ id: 'b2', game: 'orbitgolf' })];
    const { clock } = await armA();
    const field = screen.getByPlaceholderText('Wallet passphrase') as HTMLInputElement;
    fireEvent.change(field, { target: { value: 'hunter2' } });
    fireEvent.click(screen.getByText(/^Approve 0\.05 DCR/));
    await screen.findByText(/wrong passphrase/);
    expect((screen.getByPlaceholderText('Wallet passphrase') as HTMLInputElement).value).toBe('');
    expect(vi.mocked(decideGamingSpend)).toHaveBeenCalledWith('a1', true, 'hunter2');
    clock.mockRestore();
  });
});

describe('payout passphrase', () => {
  it('leaves the dialog field empty while the approval is still on its way', async () => {
    let settle: () => void = () => {};
    vi.mocked(approveGamingPayout).mockReturnValueOnce(new Promise((resolve) => { settle = () => resolve({}); }));
    payouts = [payout({ id: 'tx9' })];
    render(<GamingApprovalsPanel />);
    fireEvent.click(await screen.findByText('Review payout'));
    const dialog = await screen.findByRole('dialog');
    const field = dialog.querySelector('input[type="password"]') as HTMLInputElement;
    fireEvent.change(field, { target: { value: 'hunter2' } });
    fireEvent.click(screen.getByText('Approve payout'));
    expect(vi.mocked(approveGamingPayout)).toHaveBeenCalledWith('tx9', 'hunter2');
    expect((dialog.querySelector('input[type="password"]') as HTMLInputElement).value).toBe('');
    settle();
  });
});
