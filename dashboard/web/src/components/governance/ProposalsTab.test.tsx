// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Proposal, ProposalsResponse } from '../../services/api';
import * as api from '../../services/api';
import { ProposalsTab } from './ProposalsTab';

vi.mock('../../services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api')>()),
  getProposals: vi.fn(),
  refreshProposals: vi.fn(),
}));
vi.mock('./VoteTrickleCard', () => ({ VoteTrickleCard: () => <div>vote trickle</div> }));

const row = (token: string, voteStatus: string, archived: boolean): Proposal => ({
  token, name: `Proposal ${token}`, username: 'author', status: 'finished', voteStatus,
  voteCounts: {}, totalVotes: 0, quorumMin: 0, eligibleTickets: 0, endBlock: 0, blocksLeft: 0,
  currentChoice: '', votedTicketCount: 0, archived,
});
const answer = (proposals: Proposal[], offline?: ProposalsResponse['offline']): ProposalsResponse => ({
  proposals, fetchedAt: 0, refreshAvailableAt: 0, hasMore: false, offline,
});
const httpError = (status: number) =>
  Object.assign(new Error('failed'), { response: { status, data: 'upstream 500' } });

const show = () =>
  render(
    <MemoryRouter>
      <ProposalsTab />
    </MemoryRouter>,
  );
const pick = (bucket: string) =>
  fireEvent.change(screen.getByRole('combobox'), { target: { value: bucket } });

afterEach(() => {
  cleanup();
  vi.mocked(api.getProposals).mockReset();
  vi.mocked(api.refreshProposals).mockReset();
});

describe('ProposalsTab archive', () => {
  it('keeps the archived buckets reachable while Politeia is disabled', async () => {
    vi.mocked(api.getProposals).mockImplementation(async (status) => {
      if (status === 'voting') throw httpError(503);
      return answer([row('aaaa1111', 'approved', true)], 'disabled');
    });
    show();
    await screen.findByText('Politeia is disabled in Settings');
    expect(screen.queryByText('vote trickle')).toBeNull();

    pick('finished');
    await screen.findByText('Proposal aaaa1111');
    expect(screen.queryByText('Politeia is disabled in Settings')).toBeNull();
    expect(screen.getByText(/^Politeia is disabled in Settings\. Showing the archived/)).toBeTruthy();
    expect(screen.getByText('archived')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /refresh/i })).toBeNull();
    expect(screen.queryByText('vote trickle')).toBeNull();
  });

  it('serves the archive when Politeia does not answer, keeping refresh', async () => {
    vi.mocked(api.getProposals).mockImplementation(async (status) => {
      if (status === 'voting') throw httpError(502);
      const rows = [row('bbbb2222', 'ineligible', true), row('cccc3333', 'ineligible', false)];
      return answer(rows, 'unreachable');
    });
    show();
    await screen.findByText('upstream 500');
    expect(screen.queryByText('0 proposals')).toBeNull();

    pick('abandoned');
    await screen.findByText('Proposal bbbb2222');
    expect(screen.queryByText('upstream 500')).toBeNull();
    expect(screen.getByText(/^Politeia did not answer\. Showing the archived/)).toBeTruthy();
    expect(screen.getAllByText('archived')).toHaveLength(1);
    expect(screen.getByRole('button', { name: /refresh/i })).toBeTruthy();
    expect(screen.getAllByText('ineligible')[0].className).toContain('text-destructive');

    pick('voting');
    await screen.findByText('upstream 500');
    expect(screen.queryByText(/Showing the archived/)).toBeNull();
  });

  it('drops the archive note once a refresh reaches Politeia', async () => {
    const archived = [row('bbbb2222', 'approved', true)];
    vi.mocked(api.getProposals).mockResolvedValue(answer(archived, 'unreachable'));
    vi.mocked(api.refreshProposals).mockResolvedValue(answer(archived));
    show();
    await screen.findByText(/^Politeia did not answer\./);
    fireEvent.click(screen.getByRole('button', { name: /refresh/i }));
    await waitFor(() => expect(screen.queryByText(/Showing the archived/)).toBeNull());
    expect(screen.getByText('Proposal bbbb2222')).toBeTruthy();
  });

  it('shows no archive note on a live list', async () => {
    vi.mocked(api.getProposals).mockResolvedValue(answer([row('dddd4444', 'started', false)]));
    show();
    await screen.findByText('Proposal dddd4444');
    expect(screen.queryByText(/Showing the archived/)).toBeNull();
    expect(screen.queryByText('archived')).toBeNull();
    expect(screen.getByText('vote trickle')).toBeTruthy();
  });
});
