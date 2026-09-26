import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { GamingPayout } from '../../services/gamingApi';
import { GamingPayoutApprovals, PayoutShare } from './GamingPayoutApprovals';

let rows: GamingPayout[] = [];
vi.mock('../../hooks/useVisiblePoll', () => ({ useVisiblePoll: (fn: () => void) => fn() }));
vi.mock('../../services/gamingApi', () => ({
  getGamingPayouts: async () => rows,
  approveGamingPayout: vi.fn(),
  rejectGamingPayout: vi.fn(),
  sendGamingPayoutSignatures: (id: string) => { sent.push(id); return Promise.resolve({}); },
}));
const sent: string[] = [];

afterEach(cleanup);

const payout = (over: Partial<GamingPayout>): GamingPayout => ({
  id: 'tx1', table: 'table1', scope: { game: 'poker' }, state: 'awaiting_approval',
  payments: [{ key: '02aa', atoms: 199990000 }],
  destinations: { '02aa': 'DsOurs', '02bb': 'DsTheirs' },
  feeAtoms: 10000, expiresAt: Date.now() / 1000 + 600, signatures: {},
  mine: { key: '02aa', address: 'DsOurs', stakeAtoms: 100000000, receiveAtoms: 199990000 },
  ...over,
});

describe('GamingPayoutApprovals', () => {
  it("marks the operator's output and states stake, payout and net", async () => {
    rows = [payout({})];
    render(<GamingPayoutApprovals />);
    expect(await screen.findByText('You')).toBeTruthy();
    expect(screen.getByText('DsOurs').parentElement?.textContent).toContain('You');
    const line = screen.getByText(/You staked/).textContent;
    expect(line).toContain('1 DCR');
    expect(line).toContain('1.9999 DCR');
    expect(line).toContain('net +0.9999 DCR');
  });
});

describe('signature delivery', () => {
  it('offers one send only when Bison Relay never took our signatures', async () => {
    sent.length = 0;
    rows = [payout({ id: 'unsent', state: 'awaiting_signatures', signaturesSent: 'unsent' })];
    render(<GamingPayoutApprovals />);
    expect(await screen.findByText(/did not reach Bison Relay/)).toBeTruthy();
    fireEvent.click(screen.getByText('Send signatures'));
    await waitFor(() => expect(sent).toEqual(['unsent']));
  });
  it('says an unknown outcome plainly', async () => {
    rows = [payout({ state: 'awaiting_signatures', signaturesSent: 'uncertain' })];
    render(<GamingPayoutApprovals />);
    expect(await screen.findByText(/could not be confirmed/)).toBeTruthy();
  });
  it('shows nothing once sent', async () => {
    rows = [payout({ state: 'awaiting_signatures', signaturesSent: 'sent' })];
    render(<GamingPayoutApprovals />);
    expect(await screen.findByText('You')).toBeTruthy();
    expect(screen.queryByText('Send signatures')).toBeNull();
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
