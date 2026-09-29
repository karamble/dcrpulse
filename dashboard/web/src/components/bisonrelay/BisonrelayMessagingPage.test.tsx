// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { BisonrelayLiveProvider } from './BisonrelayLiveProvider';
import { BisonrelayMessagingPage } from './BisonrelayMessagingPage';
import * as api from '../../services/bisonrelayApi';

vi.mock('../../hooks/useWalletReady', () => ({ useWalletReady: () => ({ isWatchOnly: false }) }));
vi.mock('../../services/bisonrelayApi', async (original) => ({
  ...(await original<typeof import('../../services/bisonrelayApi')>()),
  getBisonrelayContacts: vi.fn(),
  listBisonrelayGCs: vi.fn(async () => []),
  listBisonrelayGCInvites: vi.fn(async () => ({ invites: [], blocked_reinvites: [] })),
  getBisonrelayIdentity: vi.fn(async () => ({ identity: 'ee'.repeat(32), nick: 'me' })),
  getCommunityJoin: vi.fn(async () => null),
  getBisonrelayMessages: vi.fn(),
  getBisonrelayContactGroups: vi.fn(async () => ({ groups: [], contacts: {} })),
}));

class FakeWS {
  onopen: unknown;
  onmessage: unknown;
  onclose: unknown;
  onerror: unknown;
  close() {}
}
(globalThis as any).WebSocket = FakeWS;
(Element.prototype as any).scrollIntoView = () => {};
const mem: Record<string, string> = {};
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (k: string) => mem[k] ?? null,
    setItem: (k: string, v: string) => {
      mem[k] = v;
    },
    removeItem: (k: string) => {
      delete mem[k];
    },
  },
});
afterEach(cleanup);

const A = 'aa'.repeat(32);
const B = 'bb'.repeat(32);
const contact = (uid: string, nick: string) => ({ id: { identity: uid, nick } });
const history = (uid: string, message: string, from: string) =>
  ({ uid, page: 0, page_size: 100, entries: [{ message, from, timestamp: 1, internal: false }] }) as any;

// FEBR-6: alice's slow history must not fill bob's thread once bob is open.
it('keeps a late history reply out of the thread opened after it', async () => {
  vi.mocked(api.getBisonrelayContacts).mockResolvedValue([contact(A, 'alice'), contact(B, 'bob')] as any);
  let releaseA!: (v: any) => void;
  vi.mocked(api.getBisonrelayMessages).mockImplementation(async (uid: string) =>
    uid === A
      ? new Promise((res) => {
          releaseA = res;
        })
      : history(uid, 'BOB-HISTORY', 'bob'),
  );
  render(
    <BisonrelayLiveProvider>
      <BisonrelayMessagingPage ownNick="me" />
    </BisonrelayLiveProvider>,
  );
  await act(async () => {});
  fireEvent.click(await screen.findByText('alice'));
  fireEvent.click(screen.getByText('bob'));
  await screen.findByText('BOB-HISTORY');

  await act(async () => releaseA(history(A, 'ALICE-SECRET', 'alice')));
  expect(screen.queryByText('ALICE-SECRET')).toBeNull();
  expect(screen.queryByText('BOB-HISTORY')).not.toBeNull();
});
