// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProposalVoteCheck } from '../../services/api';
import * as api from '../../services/api';
import * as files from '../../utils/files';
import { VoteCheckCard } from './VoteCheckCard';

vi.mock('../../services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api')>()),
  startProposalVoteCheck: vi.fn(),
  getProposalVoteCheck: vi.fn(),
  downloadProposalVotes: vi.fn(),
}));
vi.mock('../../utils/files', () => ({ downloadBlob: vi.fn() }));
let pollFn: (() => void) | null = null;
vi.mock('../../hooks/useVisiblePoll', () => ({
  useVisiblePoll: (fn: () => void, _ms: number, opts?: { enabled?: boolean }) => {
    pollFn = opts?.enabled === false ? null : fn;
  },
}));

const token = 'cdcdcdcdcdcdcdcd';
const now = Math.floor(Date.now() / 1000);
const idle = { running: false, done: 0, total: 0 };
const check = (over: Partial<ProposalVoteCheck> = {}): ProposalVoteCheck => ({
  ballots: 6, counted: 2, counts: { yes: 1, no: 1 }, tallyMatches: true, checkedAt: now, ...over,
});

afterEach(() => {
  cleanup();
  pollFn = null;
  vi.mocked(api.startProposalVoteCheck).mockReset();
  vi.mocked(api.getProposalVoteCheck).mockReset();
  vi.mocked(api.downloadProposalVotes).mockReset();
  vi.mocked(files.downloadBlob).mockReset();
});

describe('VoteCheckCard', () => {
  it('runs a check, shows its progress and reloads when it ends', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue(idle);
    vi.mocked(api.startProposalVoteCheck).mockResolvedValue({ running: true, done: 0, total: 0 });
    const onChecked = vi.fn();
    render(<VoteCheckCard token={token} tally={{ yes: 1, no: 1 }} onChecked={onChecked} />);
    expect(screen.queryByRole('button', { name: /download votes/i })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Verify votes' }));
    await screen.findByText('Fetching the ballots from Politeia...');
    expect(api.startProposalVoteCheck).toHaveBeenCalledWith(token);

    vi.mocked(api.getProposalVoteCheck).mockResolvedValue({ running: true, done: 3, total: 6 });
    await act(async () => pollFn?.());
    expect(screen.getByText('Checking ballots, 3 of 6...')).toBeTruthy();
    expect(onChecked).not.toHaveBeenCalled();

    vi.mocked(api.getProposalVoteCheck).mockResolvedValue({ running: false, done: 6, total: 6 });
    await act(async () => pollFn?.());
    expect(onChecked).toHaveBeenCalledTimes(1);
    expect(pollFn).toBeNull();
  });

  it('reloads after a check that ends before the first poll', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue(idle);
    vi.mocked(api.startProposalVoteCheck).mockResolvedValue({ running: true, done: 0, total: 0 });
    const onChecked = vi.fn();
    render(<VoteCheckCard token={token} tally={{}} onChecked={onChecked} />);
    fireEvent.click(screen.getByRole('button', { name: 'Verify votes' }));
    await screen.findByText('Fetching the ballots from Politeia...');
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue({ running: false, done: 2, total: 2 });
    await act(async () => pollFn?.());
    expect(onChecked).toHaveBeenCalledTimes(1);
  });

  it('says why a check stopped and does not reload', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue({ running: true, done: 1, total: 6 });
    const onChecked = vi.fn();
    render(<VoteCheckCard token={token} tally={{}} onChecked={onChecked} />);
    await screen.findByText('Checking ballots, 1 of 6...');
    const error = 'ticket 1111 on this node: no transaction index';
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue({ running: false, done: 1, total: 6, error });
    await act(async () => pollFn?.());
    expect(screen.getByText(`The check stopped: ${error}`)).toBeTruthy();
    expect(onChecked).not.toHaveBeenCalled();
  });

  it('shows a result, the uncounted ballots and a differing tally', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue(idle);
    const problems = [{ ticket: '33'.repeat(32), problem: 'ticket was not eligible' }];
    render(
      <VoteCheckCard
        token={token}
        check={check({ tallyMatches: false, problems })}
        tally={{ yes: 9000, no: 1 }}
        onChecked={() => {}}
      />,
    );
    expect(screen.getByText(/2 of 6 ballots verified; the count differs from Politeia's tally/))
      .toBeTruthy();
    expect(screen.getByText('yes: counted 1, Politeia 9000')).toBeTruthy();
    expect(screen.getByText('1 ballots not counted:')).toBeTruthy();
    expect(screen.getByText(/: ticket was not eligible/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Verify votes again' })).toBeTruthy();
  });

  it('says when the count matches and downloads the ballots', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue(idle);
    const blob = new Blob(['{}']);
    type Download = Awaited<ReturnType<typeof api.downloadProposalVotes>>;
    vi.mocked(api.downloadProposalVotes).mockResolvedValue({ data: blob } as Download);
    const tally = { yes: 1, no: 1 };
    render(<VoteCheckCard token={token} check={check()} tally={tally} onChecked={() => {}} />);
    expect(screen.getByText(/2 of 6 ballots verified; the count matches Politeia's tally/))
      .toBeTruthy();
    expect(screen.queryByText(/counted 1, Politeia/)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /download votes/i }));
    await waitFor(() =>
      expect(files.downloadBlob).toHaveBeenCalledWith(blob, `${token}-votes.json`, 'application/json'),
    );
  });

  it('says when a check cannot start', async () => {
    vi.mocked(api.getProposalVoteCheck).mockResolvedValue(idle);
    const response = { status: 409, data: 'a vote check is already running' };
    const err = Object.assign(new Error('x'), { response });
    vi.mocked(api.startProposalVoteCheck).mockRejectedValue(err);
    render(<VoteCheckCard token={token} tally={{}} onChecked={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: 'Verify votes' }));
    expect(await screen.findByText('a vote check is already running')).toBeTruthy();
  });
});
