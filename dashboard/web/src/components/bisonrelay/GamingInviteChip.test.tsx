import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { GamingInviteChip } from './GamingInviteChip';
import { GamingChatCtx } from './gamingChatContext';
import { splitGamingInvites, type GamingInvite } from './gamingInviteParse';
import type { GamingTableStatus } from '../../services/gamingApi';

const accepted: string[] = [];
const statusAsked: string[] = [];
let tableStatus: GamingTableStatus | Error = { accepted: false, closed: false };
let statusGate: Promise<void> | null = null;
vi.mock('../../services/gamingApi', () => ({
  getGamingGames: async () => [{ id: 'poker', name: 'Poker', ready: true, minRefundBlocks: 288, bondLockBlocks: 12 }],
  acceptGamingInvite: async (_game: string, raw: string) => { accepted.push(raw); },
  getGamingTableStatus: async (game: string, sid: string) => {
    statusAsked.push(`${game}/${sid}`);
    if (statusGate) await statusGate;
    if (tableStatus instanceof Error) throw tableStatus;
    return tableStatus;
  },
}));

afterEach(() => {
  cleanup();
  accepted.length = 0;
  statusAsked.length = 0;
  tableStatus = { accepted: false, closed: false };
  statusGate = null;
});

const invite = (link: string): GamingInvite => {
  const part = splitGamingInvites(link).find((p) => p.kind === 'invite');
  if (!part || part.kind !== 'invite') throw new Error('no invite');
  return part.invite;
};
const link = 'gaming://poker/table?fv=2&sid=a1&buyin=100000000&seats=2&csv=288&until=900100&bond=50000000&bondcsv=65535&tablebond=25000000&tablebondcsv=2016';

const renderChip = (l: string) =>
  render(<GamingChatCtx.Provider value={'ab'.repeat(32)}><GamingInviteChip invite={invite(l)} /></GamingChatCtx.Provider>);

describe('GamingInviteChip', () => {
  it("shows the invite's own bond amounts and locks, not the game's defaults", async () => {
    renderChip(link);
    expect(await screen.findByText('Seat bond 0.5 DCR, locked ~227 days.')).toBeTruthy();
    expect(screen.getByText('Table bond 0.25 DCR, locked ~7 days.')).toBeTruthy();
    expect(screen.getByText('Buy-in 1 DCR, refund lock ~1 day.')).toBeTruthy();
    expect(screen.queryByText(/~1 hour/)).toBeNull();
  });

  it('joins only after the total commitment is confirmed', async () => {
    renderChip(link);
    fireEvent.click(await screen.findByText('Accept'));
    expect(accepted).toEqual([]);
    expect(screen.getByText(/up to 1\.75 DCR at this table/)).toBeTruthy();
    fireEvent.click(screen.getByText('Join table'));
    await waitFor(() => expect(accepted).toEqual([link]));
  });

  it('asks the ledger again after joining', async () => {
    renderChip(link);
    fireEvent.click(await screen.findByText('Accept'));
    fireEvent.click(screen.getByText('Join table'));
    await waitFor(() => expect(statusAsked).toEqual(['poker/a1', 'poker/a1']));
  });

  it('shows a table the ledger has accepted instead of offering Accept again', async () => {
    tableStatus = { accepted: true, closed: false, seatBond: { state: 'approved', txid: 'bb' } };
    renderChip(link);
    expect(await screen.findByText('Accepted · seat bond paid.')).toBeTruthy();
    expect(screen.queryByText('Accept')).toBeNull();
  });

  it('offers no Accept while the ledger has not answered', async () => {
    let open!: () => void;
    statusGate = new Promise((r) => { open = r; });
    tableStatus = { accepted: true, closed: false, seatBond: { state: 'approved' } };
    renderChip(link);
    await waitFor(() => expect(statusAsked).toEqual(['poker/a1']));
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText('Accept')).toBeNull();
    open();
    expect(await screen.findByText('Accepted · seat bond paid.')).toBeTruthy();
  });

  it('says the seat bond and stake are paid once both are', async () => {
    tableStatus = { accepted: true, closed: false, seatBond: { state: 'approved' }, stake: { state: 'approved' } };
    renderChip(link);
    expect(await screen.findByText('Accepted · seat bond and stake paid.')).toBeTruthy();
  });

  it('follows the payout to the chain', async () => {
    tableStatus = { accepted: true, closed: false, seatBond: { state: 'approved' }, payout: { id: 'p', state: 'publishing', chain: { state: 'mempool', confirmations: 0 } } };
    renderChip(link);
    expect(await screen.findByText('Table finished · payout in the mempool.')).toBeTruthy();
    cleanup();
    tableStatus = { accepted: true, closed: false, payout: { id: 'p', state: 'confirmed', chain: { state: 'confirmed', confirmations: 3 } } };
    renderChip(link);
    expect(await screen.findByText('Table finished · payout confirmed (3 confirmations).')).toBeTruthy();
  });

  it('passes over a payout that expired', async () => {
    tableStatus = { accepted: true, closed: true, payout: { id: 'p', state: 'expired' } };
    renderChip(link);
    expect(await screen.findByText('Table closed.')).toBeTruthy();
  });

  it('still offers Accept when the ledger cannot be read', async () => {
    tableStatus = new Error('ledger unavailable');
    renderChip(link);
    expect(await screen.findByText('Accept')).toBeTruthy();
  });

  it('offers no accept when the terms are incomplete', async () => {
    renderChip(link.replace('&bond=50000000', ''));
    expect(await screen.findByText('This invitation does not state complete terms.')).toBeTruthy();
    await waitFor(() => expect(screen.queryByText(/Checking whether/)).toBeNull());
    expect(screen.queryByText('Accept')).toBeNull();
  });
});
