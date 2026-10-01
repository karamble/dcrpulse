// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../../services/bisonrelayApi';
import { IncomingGCInvitesBanner } from './IncomingGCInvitesBanner';

vi.mock('../../../services/bisonrelayApi', () => ({
  acceptBisonrelayGCInvite: vi.fn(),
  dismissBisonrelayBlockedReinvite: vi.fn(),
  listBisonrelayGCInvites: vi.fn(),
  partBisonrelayGC: vi.fn(),
}));
vi.mock('../BisonrelayLiveProvider', () => ({
  useBisonrelayLive: () => ({ addListener: () => () => {} }),
}));

const GCID = 'dd8306131ec7e67920443c706e0f704f135a07ebf71f8ec625365a15514c8c11';
const blocked = { gcid: GCID, name: 'Decred Pulse', from: 'ab'.repeat(32), fromNick: 'aibot', count: 2, lastAttempt: '' };

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('blocked group re-invite banner', () => {
  it('Dismiss asks brclientd to forget the re-invite, not only this page', async () => {
    vi.mocked(api.listBisonrelayGCInvites).mockResolvedValue({ invites: [], blocked_reinvites: [blocked] });
    vi.mocked(api.dismissBisonrelayBlockedReinvite).mockResolvedValue();
    render(<IncomingGCInvitesBanner onAccepted={() => {}} />);
    fireEvent.click(await screen.findByTitle(/returns on the next blocked attempt/));

    expect(api.dismissBisonrelayBlockedReinvite).toHaveBeenCalledWith(GCID);
    expect(screen.queryByText(/Group re-invite blocked/)).toBeNull();
  });

  it('a failed dismiss keeps the banner hidden and says so', async () => {
    vi.mocked(api.listBisonrelayGCInvites).mockResolvedValue({ invites: [], blocked_reinvites: [blocked] });
    vi.mocked(api.dismissBisonrelayBlockedReinvite).mockRejectedValue(new Error('brclientd unreachable'));
    render(<IncomingGCInvitesBanner onAccepted={() => {}} />);
    fireEvent.click(await screen.findByTitle(/returns on the next blocked attempt/));

    expect(await screen.findByText(/brclientd unreachable|Could not dismiss/)).toBeTruthy();
    expect(screen.queryByText(/Group re-invite blocked/)).toBeNull();
  });

  it('leaving the group needs no separate dismiss', async () => {
    vi.mocked(api.listBisonrelayGCInvites).mockResolvedValue({ invites: [], blocked_reinvites: [blocked] });
    vi.mocked(api.partBisonrelayGC).mockResolvedValue();
    render(<IncomingGCInvitesBanner onAccepted={() => {}} />);
    fireEvent.click(await screen.findByRole('button', { name: /Leave/ }));

    await waitFor(() => expect(api.partBisonrelayGC).toHaveBeenCalledWith(GCID));
    expect(api.dismissBisonrelayBlockedReinvite).not.toHaveBeenCalled();
  });
});
