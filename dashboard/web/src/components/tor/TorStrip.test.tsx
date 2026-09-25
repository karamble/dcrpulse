import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { TorControl, TorStatus } from '../../services/tor/client';
import * as tor from '../../services/tor/client';
import { TorStrip, nodeWaitMessage, torStripState } from './TorStrip';

vi.mock('../../services/tor/client', async (original) => ({
  ...(await original<typeof import('../../services/tor/client')>()),
  getTorStatus: vi.fn(),
  getTorControl: vi.fn(),
}));

const status = (enabled: boolean): TorStatus => ({
  settings: { enabled, isolation: true, dcrdOnion: false, lnOnion: false, circuitLimit: 32, rev: 1 },
  proxyReachable: enabled,
  onionAddress: '',
  daemons: [
    { name: 'dcrd', running: true, tor: enabled, torRev: '1' },
    { name: 'dcrlnd', running: true, tor: enabled, torRev: '1' },
    { name: 'dcrwallet', running: true, tor: false, torRev: '1' },
  ],
});
const control = (bootstrapPct: number, bootstrapTag: string, circuits = 0): TorControl => ({
  reachable: true, bootstrapPct, bootstrapTag, circuits, bytesRead: 0, bytesWritten: 0, version: '0.4.9',
});

const show = (s: TorStatus, c: TorControl | Error) => {
  vi.mocked(tor.getTorStatus).mockResolvedValue(s);
  if (c instanceof Error) vi.mocked(tor.getTorControl).mockRejectedValue(c);
  else vi.mocked(tor.getTorControl).mockResolvedValue(c);
  return render(<MemoryRouter><TorStrip /></MemoryRouter>);
};

afterEach(cleanup);

describe('TorStrip', () => {
  it('renders nothing while Tor is off, without asking Tor', async () => {
    const { container } = show(status(false), control(100, 'done', 3));
    await waitFor(() => expect(tor.getTorStatus).toHaveBeenCalled());
    expect(container.textContent).toBe('');
    expect(tor.getTorControl).not.toHaveBeenCalled();
  });

  it('shows a connected Tor and only the services routed through it', async () => {
    show(status(true), control(100, 'done', 14));
    expect(await screen.findByText('Connected · 14 circuits')).toBeTruthy();
    expect(screen.getByText('Node')).toBeTruthy();
    expect(screen.getByText('Lightning')).toBeTruthy();
    expect(screen.queryByText('Wallet')).toBeNull();
    expect(screen.queryByRole('progressbar')).toBeNull();
  });

  it('shows bootstrap progress with its phase', async () => {
    show(status(true), control(50, 'loading_descriptors'));
    expect(await screen.findByText('Connecting 50% · loading relay descriptors')).toBeTruthy();
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('50');
  });

  it('shows Tor as not reachable when its control port does not answer', async () => {
    show(status(true), new Error('dial tcp: connection refused'));
    expect(await screen.findByText('Not reachable')).toBeTruthy();
  });
});

describe('nodeWaitMessage', () => {
  const on = (c: TorControl | null) => torStripState(status(true), c);

  it('names Tor while dcrd waits for peers behind a Tor that is not connected', () => {
    expect(nodeWaitMessage('connecting', 'Waiting for peers', on(control(50, 'loading_descriptors')))).toBe('Waiting for Tor');
    expect(nodeWaitMessage('connecting', 'Waiting for peers', on(null))).toBe('Waiting for Tor');
  });

  it('keeps the node message otherwise', () => {
    expect(nodeWaitMessage('connecting', 'Waiting for peers', on(control(100, 'done', 5)))).toBe('Waiting for peers');
    expect(nodeWaitMessage('connecting', 'Waiting for peers', torStripState(status(false), null))).toBe('Waiting for peers');
    expect(nodeWaitMessage('syncing', 'Downloading blocks', on(null))).toBe('Downloading blocks');
    const dcrdDirect = { ...on(null), routed: ['dcrlnd'] };
    expect(nodeWaitMessage('connecting', 'Waiting for peers', dcrdDirect)).toBe('Waiting for peers');
  });
});
