// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProposalDetail, ProposalVerification } from '../../services/api';
import * as api from '../../services/api';
import { ProposalDetailPage } from './ProposalDetailPage';

vi.mock('../../services/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/api')>()),
  getProposalDetail: vi.fn(),
}));
vi.mock('../../hooks/useWalletReady', () => ({ useWalletReady: () => ({ isWatchOnly: false }) }));
vi.mock('./VoteModal', () => ({ VoteModal: () => null }));
vi.mock('./VoteCheckCard', () => ({ VoteCheckCard: () => <div>vote check card</div> }));

const serverKey = 'a3a3b37951a1dd5de6ab2a35e19e266b6df2daad39aad4c2da04034ca95672f5';
const authorKey = 'b'.repeat(64);
const adminKey = 'c'.repeat(64);

const detail = (verification?: ProposalVerification): ProposalDetail => ({
  token: '1111111111111111', name: 'Marketing', username: 'author', status: 'finished',
  voteStatus: 'approved', voteCounts: { yes: 9000, no: 1000 }, totalVotes: 10000, quorumMin: 8000,
  eligibleTickets: 40000, endBlock: 800, blocksLeft: 0, currentChoice: '', votedTicketCount: 0,
  archived: !!verification, description: 'Text', submittedAt: 0, voteOptions: [], comments: [],
  verification,
});

const show = async (d: ProposalDetail) => {
  vi.mocked(api.getProposalDetail).mockResolvedValue({
    detail: d,
    fetchedAt: 1800000000,
    refreshAvailableAt: 0,
  });
  render(
    <MemoryRouter initialEntries={[`/wallet/governance/proposals/${d.token}`]}>
      <Routes>
        <Route path="/wallet/governance/proposals/:token" element={<ProposalDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
  await screen.findByText('Marketing');
};

afterEach(() => {
  cleanup();
  vi.mocked(api.getProposalDetail).mockReset();
});

describe('ProposalDetailPage verification', () => {
  it('lists what the signatures cover on an archived proposal', async () => {
    await show(
      detail({
        serverKey, authorKey, authorKeyBound: true, migrated: false, comments: 2, commentsLeftOut: 1,
        authorized: true, voteStartKey: adminKey, billingStatus: 'completed', source: 'import', archivedAt: 1800000000,
      }),
    );
    expect(screen.getByText('Signatures')).toBeTruthy();
    expect(screen.getByText(serverKey)).toBeTruthy();
    expect(screen.getByText(authorKey)).toBeTruthy();
    expect(screen.getByText(adminKey)).toBeTruthy();
    expect(screen.getByText(/2 verified, each with its signature and server receipt; 1 left out/))
      .toBeTruthy();
    expect(screen.getByText('signed by the key that authorized the vote, with a server receipt'))
      .toBeTruthy();
    expect(screen.getByText('usernames, the vote tally, statuses, dates and comment votes')).toBeTruthy();
    expect(screen.getByText('completed, signed by an admin, with a server receipt')).toBeTruthy();
    expect(screen.getByText('Tally reported by Politeia, not signed.')).toBeTruthy();
    expect(screen.getByText(/^Fetched from Politeia on .*, arrived with an imported archive file\.$/)).toBeTruthy();
    expect(screen.getByText(/^archived /)).toBeTruthy();
    expect(screen.getByText('vote check card')).toBeTruthy();
    expect(screen.getByText('No comments on this proposal yet.')).toBeTruthy();
  });

  it('calls an author key self-asserted unless it authorized the vote', async () => {
    await show(
      detail({
        serverKey, authorKey, authorKeyBound: false, migrated: false, comments: 0, commentsLeftOut: 0,
        authorized: false, source: 'import', archivedAt: 1800000000,
      }),
    );
    expect(screen.getByText(authorKey)).toBeTruthy();
    expect(screen.getByText('signed by a key the record carries itself; nothing binds it to the author'))
      .toBeTruthy();
    expect(screen.queryByText(/authorized the vote/)).toBeNull();
  });

  it('says what a 2021 migration left unsigned', async () => {
    const legacy = '27f87171d98b7923a1bd2bee6affed929fa2d2a6e178b5c80a9971a92a5c7f50';
    await show(
      detail({
        serverKey, authorKeyBound: false, migrated: true, legacyToken: legacy, comments: 0, commentsLeftOut: 0,
        authorized: false, source: 'snapshot', archivedAt: 1800000000,
      }),
    );
    expect(screen.getByText(/original signature lost when Politeia migrated this proposal in 2021/))
      .toBeTruthy();
    expect(screen.getByText(`old token ${legacy}`)).toBeTruthy();
    expect(screen.getByText('not archived: the 2021 migration left them without signatures'))
      .toBeTruthy();
    expect(screen.queryByText('Vote start:')).toBeNull();
    expect(screen.queryByText('vote check card')).toBeNull();
    expect(screen.getByText('Comments of proposals migrated in 2021 are not archived.')).toBeTruthy();
    expect(screen.getByText(/, arrived with the snapshot shipped with dcrpulse\.$/)).toBeTruthy();
  });

  it('dates a copy the dashboard fetched itself without a source note', async () => {
    await show(
      detail({
        serverKey, authorKeyBound: false, migrated: true, comments: 0, commentsLeftOut: 0,
        authorized: false, source: 'refresh', archivedAt: 1800000000,
      }),
    );
    expect(screen.getByText(/^Fetched from Politeia on [^,]*\.$/)).toBeTruthy();
  });

  it('shows no verification on a live proposal', async () => {
    await show(detail());
    expect(screen.queryByText('Signatures')).toBeNull();
    expect(screen.queryByText('Tally reported by Politeia, not signed.')).toBeNull();
    expect(screen.queryByText(/did not verify/)).toBeNull();
    expect(screen.getByText(/^updated /)).toBeTruthy();
  });

  it('says when Politeia sent a proposal whose signatures did not verify', async () => {
    await show({ ...detail(), verifyError: 'signatures did not verify: invalid signature' });
    expect(
      screen.getByText(/did not verify, so it was not archived.*: signatures did not verify: invalid/),
    ).toBeTruthy();
    expect(screen.queryByText('Signatures')).toBeNull();
  });
});
