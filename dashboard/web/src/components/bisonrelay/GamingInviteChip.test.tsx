import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { GamingInviteChip } from './GamingInviteChip';
import { GamingChatCtx } from './gamingChatContext';
import { splitGamingInvites, type GamingInvite } from './gamingInviteParse';

const accepted: string[] = [];
vi.mock('../../services/gamingApi', () => ({
  getGamingGames: async () => [{ id: 'poker', name: 'Poker', ready: true, minRefundBlocks: 288, bondLockBlocks: 12 }],
  acceptGamingInvite: async (_game: string, raw: string) => { accepted.push(raw); },
}));

afterEach(() => { cleanup(); accepted.length = 0; });

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

  it('offers no accept when the terms are incomplete', async () => {
    renderChip(link.replace('&bond=50000000', ''));
    expect(await screen.findByText('This invitation does not state complete terms.')).toBeTruthy();
    await waitFor(() => expect(screen.queryByText(/Checking whether/)).toBeNull());
    expect(screen.queryByText('Accept')).toBeNull();
  });
});
