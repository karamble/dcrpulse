import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { JoinDecredPulseModal } from './BisonrelayMessagingPage';
import { joinDecredPulse, getCommunityJoin, acceptCommunityJoin, acceptBisonrelayGCInvite } from '../../services/bisonrelayApi';

vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...await original<typeof import('../../services/bisonrelayApi')>(),
  joinDecredPulse: vi.fn(), getCommunityJoin: vi.fn(), acceptCommunityJoin: vi.fn(),
  acceptBisonrelayGCInvite: vi.fn(),
}));
const target = { id: 'server-attempt', botUID: '1'.repeat(64), gcid: '2'.repeat(64), status: 'waiting' as const, joined: false };
beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(joinDecredPulse).mockResolvedValue(target);
  vi.mocked(getCommunityJoin).mockResolvedValue(target);
  vi.mocked(acceptCommunityJoin).mockResolvedValue();
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.resetAllMocks(); });
async function start() {
  await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'Join Decred Pulse' })); });
}
it('routes delayed acceptance through the bound server attempt, never generic IID acceptance', async () => {
  const joined = vi.fn();
  render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={joined} />);
  await start();
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(acceptCommunityJoin).toHaveBeenCalledWith(target.id);
  expect(acceptBisonrelayGCInvite).not.toHaveBeenCalled();
  expect(joined).not.toHaveBeenCalled();
  vi.mocked(getCommunityJoin).mockResolvedValue({ ...target, joined: true });
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(joined).toHaveBeenCalledOnce();
  expect(screen.getByText('You have joined Decred Pulse.')).toBeTruthy();
});
it('does not accept when closed during an outstanding status read', async () => {
  let resolve!: (value: typeof target) => void;
  vi.mocked(getCommunityJoin).mockReturnValue(new Promise(r => { resolve = r; }));
  const view = render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={vi.fn()} />);
  await start();
  await act(async () => { vi.advanceTimersByTime(3000); });
  view.unmount();
  await act(async () => { resolve(target); });
  expect(acceptCommunityJoin).not.toHaveBeenCalled();
});
it('rejects a stale response for another join', async () => {
  vi.mocked(getCommunityJoin).mockResolvedValue({ ...target, id: 'another' });
  render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={vi.fn()} />);
  await start();
  await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
  expect(acceptCommunityJoin).not.toHaveBeenCalled();
  expect(screen.getByText(/bot or local identity changed/)).toBeTruthy();
});
it('keeps legacy bots on the explicit manual path', async () => {
  vi.mocked(joinDecredPulse).mockResolvedValue({ id: 'legacy', status: 'manual', joined: false });
  render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={vi.fn()} />);
  await start();
  await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
  expect(screen.getByText(/does not provide community identity/)).toBeTruthy();
  expect(acceptCommunityJoin).not.toHaveBeenCalled();
  expect(acceptBisonrelayGCInvite).not.toHaveBeenCalled();
  vi.mocked(joinDecredPulse).mockResolvedValue(target);
  await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'Request a new invite' })); });
  expect(joinDecredPulse).toHaveBeenLastCalledWith(true);
});
it('resumes the existing saved attempt after reopening without requesting a replacement', async () => {
  const view = render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={vi.fn()} />);
  await start(); view.unmount();
  render(<JoinDecredPulseModal onClose={vi.fn()} onJoined={vi.fn()} />);
  await start();
  expect(joinDecredPulse).toHaveBeenNthCalledWith(1, false);
  expect(joinDecredPulse).toHaveBeenNthCalledWith(2, false);
});
