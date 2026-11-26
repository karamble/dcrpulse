// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import type { LightningPeer, LightningPeers } from '../../services/lightningApi';
import { getLightningPeers, reconnectLightningPeer } from '../../services/lightningApi';
import { PeersCard } from './PeersCard';

vi.mock('../../services/lightningApi', () => ({
  getLightningPeers: vi.fn(),
  reconnectLightningPeer: vi.fn(),
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const nowSec = () => Math.floor(Date.now() / 1000);

const peer = (over: Partial<LightningPeer>): LightningPeer => ({
  pubkey: '02' + 'aa'.repeat(32),
  address: '198.51.100.1:9735',
  inbound: false,
  tor: false,
  pingMicros: 0,
  bytesSent: 0,
  bytesRecv: 0,
  atomsSent: 0,
  atomsRecv: 0,
  hasChannel: false,
  ...over,
});

const renderWith = async (data: LightningPeers) => {
  vi.mocked(getLightningPeers).mockResolvedValue(data);
  await act(async () => {
    render(<PeersCard />);
  });
};

it('lists connected peers with the channel tag, Tor marker and last error', async () => {
  await renderWith({
    peers: [
      peer({ pubkey: '02' + 'b1'.repeat(32), alias: 'hub', tor: true, hasChannel: true, pingMicros: 42000,
        lastError: 'peer said no', lastErrorUnix: nowSec() - 60 }),
      peer({ pubkey: '02' + 'b2'.repeat(32), alias: 'gossip' }),
    ],
    offlineChannelPeers: [],
  });
  expect(screen.getByText('2 connected')).toBeTruthy();
  expect(screen.getByText('hub')).toBeTruthy();
  expect(screen.getByText('gossip')).toBeTruthy();
  expect(screen.getAllByText('channel')).toHaveLength(1);
  expect(screen.getAllByTitle('Connected over Tor')).toHaveLength(1);
  expect(screen.getByText('42 ms')).toBeTruthy();
  expect(screen.getByText(/peer said no/)).toBeTruthy();
  expect(screen.queryByText('Channel peers not connected')).toBeNull();
});

it('reconnects an offline channel peer and reloads the list', async () => {
  const away = '02' + 'c1'.repeat(32);
  await renderWith({
    peers: [],
    offlineChannelPeers: [
      { pubkey: away, alias: 'away', addresses: ['198.51.100.9:9735'], channels: 2 },
      { pubkey: '02' + 'c2'.repeat(32), alias: 'lost', addresses: [], channels: 1 },
    ],
  });
  expect(screen.getByText('No peers connected')).toBeTruthy();
  expect(screen.getByText('Channel peers not connected')).toBeTruthy();
  expect(screen.getByText('No address in the channel graph')).toBeTruthy();
  const buttons = screen.getAllByRole('button', { name: 'Reconnect' }) as HTMLButtonElement[];
  expect(buttons.map((b) => b.disabled)).toEqual([false, true]);

  vi.mocked(reconnectLightningPeer).mockResolvedValue();
  await act(async () => {
    fireEvent.click(buttons[0]);
  });
  expect(reconnectLightningPeer).toHaveBeenCalledTimes(1);
  expect(reconnectLightningPeer).toHaveBeenCalledWith(away);
  expect(getLightningPeers).toHaveBeenCalledTimes(2);
});

it('shows a failed reconnect on that peer only', async () => {
  await renderWith({
    peers: [],
    offlineChannelPeers: [
      { pubkey: '02' + 'd1'.repeat(32), alias: 'first', addresses: ['198.51.100.2:9735'], channels: 1 },
      { pubkey: '02' + 'd2'.repeat(32), alias: 'second', addresses: ['198.51.100.3:9735'], channels: 1 },
    ],
  });
  vi.mocked(reconnectLightningPeer).mockRejectedValue({ response: { data: 'ConnectPeer: handshake failed' } });
  await act(async () => {
    fireEvent.click(screen.getAllByRole('button', { name: 'Reconnect' })[1]);
  });
  const msg = screen.getByText('ConnectPeer: handshake failed');
  expect(msg.closest('div')?.textContent).toContain('second');
  expect(msg.closest('div')?.textContent).not.toContain('first');
  expect(getLightningPeers).toHaveBeenCalledTimes(1);
});
